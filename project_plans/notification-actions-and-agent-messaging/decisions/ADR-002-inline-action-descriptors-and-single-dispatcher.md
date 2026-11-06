# ADR-002: Inline actions are server-computed descriptors dispatched through one `InvokeNotificationAction` RPC

**Status**: Accepted (2026-10-18, owner defaults accepted at the Phase 5 start gate); prior status: Proposed (2026-10-17); triad round 2: this ADR becomes Accepted when the owner-defaults list in plan `Unresolved Questions` is confirmed at the Phase 5 start gate (one explicit accept step for all eight ADRs; until then the stated defaults proceed)
**Project**: notification-actions-and-agent-messaging
**Flag**: `notification_inline_actions`

## Context

Notification rows today only offer Approve/Deny (approvals), mark read/dismiss, and open-session; backlog rows link out; guidance and pending-question answers exist only in session/item detail views (`research/features.md` sections 1.2-1.4, gaps 1, 10). Three different answer mechanisms already exist and each carries its own safety checks: `ResolveApproval`, `AnswerGuidanceRequest`, `ReplyToPendingQuestion` (claim, `DialogMatch`, write lease, audit), plus backlog transition/verdict RPCs. The `OmnibarAction` union + `dispatch.ts` + registry is the repo convention for a closed set of user-triggerable actions.

## Decision

1. The server computes `repeated NotificationActionDescriptor actions` for each notification record **at read time from the underlying entity** (pending approval, GuidanceRequest, pending question, backlog item state). Descriptors are never stored on the history row. A descriptor carries `kind` (`ActionKind` enum, `*_UNSPECIFIED = 0`), label, a typed params schema (options for answers, verdict choices for backlog decisions), `requires_confirm`, `tier` (see ADR-004) and the entity key (`question_id`, `guidance_request_id`, `approval_id`, `item_id`) the action targets. `question_id` is the `PendingQuestionStore` key of a live prompt that has no durable row yet; once the prompt is mirrored to a `GuidanceRequest` the descriptor carries `guidance_request_id` instead, and the notification metadata carries both. They name two id spaces and are never interchangeable. The history record ID is **never** the action key (collapse updates record IDs; pitfalls section 8).
2. One RPC, `InvokeNotificationAction{action_kind, target_key, params, idempotency_key}`, returns a typed `ActionOutcome` enum (`APPLIED`, `ALREADY_RESOLVED`, `STALE`, `NOT_SENT`, `SEND_INDETERMINATE`, `RATE_LIMITED`, `FORBIDDEN`, `FLAG_OFF`, `INVALID`), never a Connect error for expected conditions (ADR-010 precedent).
3. The dispatcher is thin: flag check, idempotency (payload-hash-checked, same shape as the hidden-reply cache), metrics, audit, then **delegation to the existing handler** which keeps its own approval-gate and caller-identity checks. The dispatcher does not re-implement safety.
4. The entity is acted on first; the history row is resolved afterwards by event (in spirit the `resolved_item_reaper` pattern, but the existing reaper is polled and item-scoped, keyed on `metadata["item_id"]` with an `ItemStateLookup`, `server/notifications/resolved_item_reaper.go:28-62`; keying on `question_id`/`approval_id` and driving it from a dispatcher-published resolve event is new design, Tasks 2.7a/2.7c), never the reverse.
4a. Two additional action kinds, `CANCEL_WAKE` and `CANCEL_PEER_HELP` (Tier A), are added by Epic 4 so a stalled peer request and a pending wake can be cancelled from the pane. Descriptors are assembled with a `ListenerClass` taken from the request context (Task 2.2d) so Tier B is `available=false` on the remote listener.
5. Clients render only from descriptors; an unknown `kind` degrades to `OPEN` (open session/item). The TS side is one `ActionRow` component and one `useInvokeNotificationAction` hook; the `OmnibarAction`-style triple (type, dispatch, test) is kept in sync via the registry doc.
6. No new notification enum values are introduced for decisions. Backlog decision notifications reuse `CONFIRMATION_NEEDED`/`INPUT_REQUIRED` (needs-human class) so the delivery gate keeps them for hidden-session items; a table test asserts every notification emission added by this project maps to a non-routine class where needed (`server/deliverygate/policy.go` exhaustiveness test stays authoritative for enum values).

## Consequences

- One idempotency/audit/metrics path for all inline actions; adding a new action type is: new `ActionKind`, descriptor builder case, delegate. Closed set, compiler-enforced `switch`.
- Stale actions return `ALREADY_RESOLVED`/`STALE` and the UI refetches instead of toasting an error.
- Descriptor computation adds a lookup per history page; bounded (hundreds of records, tens of sessions) and cached per request.

## Alternatives rejected

- **Per-type RPCs called directly from the client** (what exists): each new action forks UI and error handling; no shared idempotency; jscpd pressure from copy-pasted hooks.
- **Store actions on the history row**: goes stale when the entity changes, binds to a collapsed record ID.
- **Generic dispatcher that re-implements checks**: would bypass the approval/identity checks that live in the handlers (top pitfall 1).
