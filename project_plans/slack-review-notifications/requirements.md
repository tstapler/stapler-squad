# Requirements: slack-review-notifications

**Date**: 2026-08-06
**Type**: feature addition (backend integration + config + limited frontend settings)
**Source**: Migrated from https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/39 (backlog item `ac44fc3c-18ab-40ea-91d2-c81e630bd0bd`)

## Problem Statement
The review queue (`server/services/review_queue_service.go`, `session.ReviewQueue`) and the approval flow (`server/services/approval_handler.go`, endpoint `/api/hooks/permission-request`) only surface pending items in the web dashboard. A user who does not have the dashboard open has no way to know an agent is blocked waiting on review/approval. This turns the review queue into a pull interface (must remember to check) instead of a push interface (get notified, act immediately), which slows down unblocking agents — especially from mobile, where the dashboard isn't always convenient to open.

## Baseline
Today: `NotificationService` (`server/services/notification_service.go`) delivers in-app/toast notifications only, scoped to a connected browser session. There is no outbound webhook integration of any kind in the codebase (confirmed: no `webhook`/`Webhook`/Slack references in `config/`, `server/services/notification_service.go`, or elsewhere). `ApprovalHandler.HandlePermissionRequest` (`server/services/approval_handler.go`) already exposes the approve/deny mechanics via HTTP that a callback-style integration could reuse, but nothing calls it from outside the web UI or MCP tools today.

## Users / Consumers
Single self-hosted user (Tyler) running stapler-squad locally/on a home server, per repo's existing single-tenant assumption (see `project_plans/backlog-stuck-item-visibility/requirements.md` for the established baseline of "single user, self-hosted instance, no multi-tenant/auth considerations beyond what exists").

## Success Metrics
- A review-queue item (session needing review) or approval-pending item (permission request) reliably produces a Slack message via a configured incoming webhook within one poll cycle of appearing.
- Verification: manually trigger a review-queue item and a permission-request approval on a locally running instance with a real (or test) Slack webhook URL configured, confirm the message arrives with session name, tool/summary, and a working link back to the dashboard.

## Appetite
Medium (1–2 weeks) — outbound notification (webhook POST) is a small, additive feature. Inbound Slack Interactive Components (Approve/Deny buttons posting back into stapler-squad) is a materially larger scope (requires Slack app registration, signing-secret verification, a new public HTTP endpoint, and — per the original issue — public reachability from Slack's servers, i.e. ngrok/reverse-proxy for a home-network single-user instance). See Scope below for the split.

**Fallback increment**: Outbound notifications alone (Phase 1) are fully shippable and valuable on their own — a user gets pushed to Slack and clicks through to the existing web UI to approve/deny. Inbound interactive buttons (Phase 2) is an independent, separately-shippable increment gated by the user actually wanting to expose the instance publicly.

## Constraints
- Must follow existing config patterns in `config/config.go` / `config/types.go` (JSON-backed `Config` struct, no new config file format).
- Single-user, self-hosted — no per-user/multi-tenant Slack workspace routing needed.
- Existing `.claude/rules/feature-registry.md` applies: new RPC(s)/settings UI need per-feature registry entries.
- Inbound approval callback (if built) must not weaken the existing approval flow's trust model — anything that can call `/api/hooks/permission-request`-equivalent effectively has agent-approval authority, so verifying the request actually came from Slack (signing secret) is a hard requirement, not optional hardening.

## Non-functional Requirements
- **Performance SLO**: not applicable — low-frequency, human-latency notifications; existing review-queue poll cadence is an acceptable trigger point.
- **Reliability**: a Slack delivery failure (webhook down, network error, rate limit) must not block or fail the underlying review-queue/approval flow — notification delivery is fire-and-forget/best-effort with logging, not a blocking dependency.
- **Security classification**: internal, but webhook URL and (if Phase 2 is built) Slack signing secret are secrets — must not be logged, must follow existing secret-handling conventions (see 1Password usage elsewhere in the repo) rather than being stored in plaintext-committed config.
- **Availability**: inbound approval callback (Phase 2) requires the instance to be reachable from the public internet, which is an explicit user-facing tradeoff (ngrok/reverse proxy), not something this feature can paper over.

## Scope
### In Scope (Phase 1 — Outbound notifications, this is the primary deliverable)
- Config fields for `slack_webhook_url`, `slack_notify_on_queue_item` (bool), and a queue-depth threshold option, added to the existing `Config` struct.
- A Slack notifier that POSTs a formatted message (session name, requested tool/summary, diff summary if available, direct link to the queue item in the dashboard) to the configured webhook when:
  - a new review-queue item arrives (if `slack_notify_on_queue_item` true), or
  - queue depth exceeds a configured threshold N (alternative/additional trigger).
- Wire the notifier into the existing review-queue poller / `NotificationService` path as an additional sink, not a replacement for in-app notifications.
- Settings UI to configure the webhook URL and toggle(s), following existing settings page conventions.
- Feature registry entries per `.claude/rules/feature-registry.md`.
- Unit tests for the notifier (message formatting, trigger conditions, failure-does-not-block behavior) and an e2e/integration test for the settings UI per existing conventions.

### In Scope (Phase 2 — Inbound approve/deny via Slack, separately gated)
- `slack_approval_enabled` config flag (default false).
- Slack interactive-component (button) payload on outbound messages when enabled.
- New endpoint to receive Slack's interactive callback, verify Slack's request signature (signing secret), and translate the click into the existing approve/deny mechanics (`ApprovalHandler`/`/api/hooks/permission-request` semantics — exact reuse-vs-new-endpoint decision left to planning).
- Documentation covering the public-reachability requirement (ngrok/reverse proxy) and its security implications, since this is a user-facing tradeoff this feature cannot eliminate.

### Out of Scope
- Any other chat platform (Telegram, Discord, etc.) — Slack only, per the issue.
- Multi-workspace or multi-user Slack routing.
- Building or documenting an ngrok/reverse-proxy setup script — the feature only needs to document the requirement, not automate exposing the instance.
- Changing the existing in-app `NotificationService` behavior for connected browser clients.

## Rabbit Holes
- Phase 2's inbound callback is the highest-risk piece: it's a new unauthenticated-by-default HTTP surface if signature verification is skipped or done wrong, and it only matters if the user actually wants public reachability. Planning should treat Phase 1 and Phase 2 as separately shippable and default to shipping Phase 1 alone first.
- "Diff summary" in the outbound message needs a size cap — Slack message/block payloads have hard size limits (block text ~3000 chars, ~50 blocks per message) that a large diff could exceed if pasted in naively.
- Rate limiting: Slack incoming webhooks are rate-limited (~1 msg/sec per webhook, informal). A queue-depth-threshold burst (many items appearing at once) could hit this — worth at least a de-dupe/coalescing note in planning, not necessarily a full backoff implementation given Medium appetite.
