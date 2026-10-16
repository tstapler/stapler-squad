# Stack Research: notification-revamp

## Verdict

No new dependency is needed for any of the 6 in-scope items. Every fix is a
change to existing Go packages (`server/notifications`, `session`,
`server/services`, `pkg/classifier`) or existing React/TypeScript components
and utilities. All backend persistence stays JSON-file-based (no ORM/DB
migration), all frontend grouping/collapsing needs are already met by
existing design-system primitives.

## Backend

### Language/build
- `go.mod`: `module github.com/tstapler/stapler-squad`, `go 1.26.6`.
- ConnectRPC: `connectrpc.com/connect v1.20.0` + `connectrpc.com/otelconnect v0.8.0`. `WatchReviewQueue` (`proto/session/v1/session.proto:70`) is a standard server-streaming RPC (`stream ReviewQueueEvent`), nothing exotic — reconciliation (item 4) can reuse the same event type (`proto/session/v1/events.proto:386-390`) to push an "auto-resolved" update; no wire-protocol change is required per the requirements' explicit out-of-scope note.
- ORM: `entgo.io/ent v0.14.5` — used for the newer backlog Stage/Transition/Gate schema (recent commits f4dbf86, 010c5fc, 8b8fc75), **not** for notifications or approvals. Those two subsystems are plain JSON files (see below) and the requirements explicitly forbid a new persistence layer, so item 1 and item 3 stay in the JSON-file world.
- Testing: `github.com/stretchr/testify v1.11.1`. Standard `go test` + testify assert/require, matching existing `TestReviewQueue*`/`TestReviewQueuePoller*` suites the constraints call out as must-not-regress.

### Notification store (item 1)
`server/notifications/store.go` — file-backed JSON, in-memory `[]*NotificationRecord` guarded by `sync.Mutex`, persisted via `saveToDisk()` (atomic write pattern consistent with the rest of the codebase). Confirmed:
- `NotificationRecord` **already has** `OccurrenceCount int` and `LastOccurredAt *time.Time` fields (store.go:44-49) — the schema for "one row with an occurrence count" already exists.
- The bug is precisely in `Append()` (store.go:138-176): `findUnreadDuplicate()` (store.go:212-219) only matches `!r.IsRead`, so once a record is marked read, the next occurrence of the same `(sessionID, notificationType)` falls through to the "no duplicate found" branch and inserts a fresh row with `OccurrenceCount = 1` instead of bumping the existing (now-read) record. Fix is localized to changing the dedup key's read-state gating, not adding infrastructure.
- Frontend already anticipates a fixed backend: `web-app/src/lib/utils/notificationGrouping.ts` computes `count = max(representative.occurrenceCount ?? 0, group.length)`, i.e., it already prefers server-side `occurrenceCount` when present and only falls back to client-side grouping "for backward compatibility." No frontend change needed for item 1 beyond validating this path once the backend is fixed.
- `MaxNotifications = 500`, `MaxNotificationAge = 7 * 24h` retention constants already in place (store.go:15-18).

### Review queue / idle ack-suppression (item 2)
- `session/review_queue_determiner.go`: Stale reasons already have ack-suppression via `inst.IsAcknowledgedAfterOutput()` (line 281, `ReviewState.IsAcknowledgedAfterOutput()` in `session/review_state.go:172-183`). Idle (line 259-262, `basicIdleThreshold = 5*time.Second`) has no equivalent check — confirms the requirements' claim exactly.
- `ReviewState` (`session/review_state.go`) is embedded in `Instance`, not separately persisted/locked — mutated only through the actor's serialized `send()`/`sendSyncErr()` closures (documented in the file's header comment), the same non-locking discipline the `instance-lock-free-reads.md` project rule describes for other `Instance` fields. Any idle-ack fix should follow this existing pattern (a new timestamp field alongside `LastAcknowledged`, routed through the actor), not a new lock or store.
- `MarkAcknowledged()` (`session/instance_state.go:246`) and `Storage.UpdateInstanceAcknowledged()` (`session/storage.go:641`) are the existing acknowledge-write paths to extend.

### Working-state detection (item 3)
- `session/detection` package already defines `StatusIdle`, `StatusReady`, `StatusProcessing`, `StatusContext` (not found as a literal — the requirements' phrasing may refer to a different constant; **only `StatusIdle` and other `Status*` values were found**, not a `StatusContext`) and related states (`detector.go`, `idle.go`, `pattern_set.go`, `osc_priority.go`). This is the same detection package referenced by the prior `review-queue-state-detection` plan. Reuse is very plausible: `detector.go` already distinguishes `StatusIdle` from `StatusProcessing`/`StatusExecuting`, which is the multi-signal building block FR-1 needs — no new package required. Flag for the planning phase: verify whether `StatusContext` is a typo/stand-in for a state not yet present, since it wasn't found by name.

### Approval rule reconciliation (item 4)
- `server/services/approval_store.go` — also JSON-file-backed (`NewApprovalStore(filePath)`, `os.WriteFile` with a tmp-file+rename atomic-write pattern at line ~335-350), matching the "no new persistence layer" constraint.
- `pkg/classifier/classifier.go`'s `RuleBasedClassifier.ReplaceRules()`/`AddRules()` (lines 421-439) are the existing rule-reload entry points (used by the already-shipped `dynamic-rule-reload` #538). Reconciliation (item 4) is a matter of, after a rule upsert/reload, iterating currently-pending `PendingApproval` records in `ApprovalStore` and re-running `Classify()` against each — both pieces already exist; no new classifier engine or store needed. The audit-note requirement ("Auto-resolved by rule: <name>") maps onto the existing `Metadata` map pattern already used for auto-approval bookkeeping (`server/notifications/store.go:198-204`, `classifier_rule_id`/`classifier_rule_name` keys) — reuse the same metadata shape rather than inventing a new one.

## Frontend (web-app/)

- Framework: **Next.js 15.3.2**, **React 19.0.0**, **TypeScript ^5.9.3** (`web-app/package.json`). Package manager is pnpm 10.27.0 (per project convention, not npm/yarn).
- RPC client: `@connectrpc/connect ^2.1.1` + `@connectrpc/connect-web ^2.1.1`, matching the backend's `connectrpc.com/connect v1.20.0` — same protocol family, current versions, nothing to upgrade.
- State/data: `@reduxjs/toolkit ^2.11.2` is the only state-management dependency present — no React Query/SWR/Zustand. Notifications/review-queue pages should follow whatever pattern existing pages already use for RPC-backed state (Redux slices/hooks) rather than introducing a new data-fetching library.
- Grouping/collapsing UI — directly reusable, already built:
  - `@radix-ui/react-accordion ^1.2.17` underlies `web-app/src/components/ui/Collapsible.tsx`, which exports `CollapsibleSection` and a `CollapsibleGroup` (shared `Accordion.Root type="multiple"` for correct keyboard roving-tabindex across sibling sections — see file header comment referencing ADR-027). This is used throughout `web-app/src/components/backlog/detail/*Section.tsx` (Activity Log, Progress History, etc.) as the standard "collapsed-by-default section" primitive.
  - `web-app/src/lib/hooks/useShowMore.ts` — a hook already used by `ActivityLogSection.tsx` to cap a list's default rendering (e.g., 8 items) with a "show more" affordance; directly applicable to "grouped/collapsed activity" sections on both target pages.
  - `web-app/src/lib/utils/notificationGrouping.ts` already implements session+type grouping with occurrence counting (see above) — the Notifications page IA reboot (item 5) can build its "needs a decision" vs. grouped-activity split on top of this existing utility rather than writing new grouping logic.
- Toast/notification UI: **no external toast library** (no `sonner`/`react-hot-toast`/`react-toastify` in `package.json`). `web-app/src/components/ui/NotificationToast.tsx` is a fully custom component (vanilla-extract styles: `toast.css.ts`, states like `visible`/`exiting`/`minimized`) driven by `notification-policy.ts` (`toastAutoCloseMs`/`toastAutoMinimizeMs`) — any toast-adjacent changes stay in this custom component, not a library integration.
- Styling: vanilla-extract (`.css.ts` files throughout, e.g. `NotificationsPage.css.ts`, `ReviewQueuePanel.css.ts`) — consistent with `docs/reference/css-architecture.md`; new IA sections should follow the same `.css.ts` convention.
- Testing: Jest `^30.2.0` (`web-app/package.json`'s `test` script). Existing suites to protect: `web-app/src/app/notifications/__tests__/NotificationsPage.test.tsx`, `web-app/src/app/review-queue/__tests__/ReviewQueue.focus.test.tsx`, `web-app/src/lib/__tests__/notification-policy.test.ts`, `web-app/src/lib/utils/notifications.test.ts`.
- Existing target pages/components confirmed present (no new files needed, only edits): `web-app/src/app/notifications/NotificationsPage.tsx`, `web-app/src/components/ui/NotificationPanel.tsx`, `web-app/src/components/ui/NotificationItem.tsx`, `web-app/src/components/sessions/ReviewQueuePanel.tsx`, `web-app/src/components/sessions/ReviewQueueBadge.tsx`.

## Open questions for the planning phase

1. Requirements text mentions a `detection` package "`StatusIdle`/`StatusContext` states" for an unrelated unattended-PTY-write feature — only `StatusIdle` was found by that literal name in `session/detection/*.go`; `StatusContext` wasn't found verbatim. Worth a direct confirmation with whoever wrote the requirements, or a broader grep in the planning phase, before assuming it's reusable as named.
2. Confirm during planning whether `ReviewState`'s actor-serialized, non-locking field-access discipline (documented in `session/review_state.go`'s header) is compatible with wherever idle-ack-suppression state needs to be read from (e.g., the ConnectRPC handler thread) — likely yes via the same accessor-method pattern the `instance-lock-free-reads.md` rule already mandates, but flag explicitly since it's a project-enforced rule with a linter-adjacent check.
