# UX Research: quota-aware-backlog-gating

Scope per requirements.md: notifications for pause/resume transitions, plus (only if
trivial) a status indicator. No new dashboard. Single-user internal tool — recommendations
below are deliberately minimal.

## 1. Precedent: how `capacity_monitor.go` alerts reach the UI

Confirmed end-to-end pipeline, already used for exactly this class of event (a backend
component making an autonomous decision that the user needs to see, not silently):

```
CapacityMonitor.handleTransitionTrigger()          (server/services/capacity_monitor.go:250-309)
  → m.eventBus.Publish(events.NewNotificationEvent(...))   (pkg/events/types.go:223)
  → server/notifications/subscriber.go                     (dedup/coalesce + persist history)
  → frontend: NotificationContext → NotificationToast (web-app/src/components/ui/NotificationToast.tsx)
                                   → NotificationPanel (bell-icon history)
```

There is a second, more directly reusable variant already wired for backlog specifically:
`session.Notifier` interface + `EventBusNotifier` adapter
(`server/services/backlog_notifier.go`), used today for per-backlog-item notifications
(e.g. auto-close, self-heal actions — per prior memory
`feedback_document_ai_decisions_in_edge_cases`). Its `Notify(itemID, title, message,
notificationType, priority)` signature calls the same `events.NewNotificationEvent`,
threading `itemID` through as the event's `sessionID` so the notification subscriber's
coalescing key (`sessionID:notificationType`, 500ms window) doesn't collide across
different items.

**Recommendation**: reuse `session.Notifier`/`EventBusNotifier` for pause/resume, not a new
event type. Use a stable synthetic ID (e.g. `"backlog-quota-gate"`) as the `itemID`/session-ID
argument so repeated pause events (if the gate re-evaluates every reconcile tick while still
paused) coalesce against each other instead of spamming. `CapacityMonitor` additionally
self-rate-limits transitions to once per 5 minutes per key
(`server/services/capacity_monitor.go:258-261`) — apply the same guard here, since the
500ms subscriber-level coalescing window is too short to prevent duplicate toasts if the
reconcile ticker fires every N minutes while quota stays low.

**Notification type mapping** (`web-app/src/lib/utils/notificationMapping.ts:17-45`):
- Pause → `NotificationType.WARNING` (maps to UI `"warning"`, ~12s toast per
  `notification-policy.ts:40`, non-actionable so not pinned).
- Resume → `NotificationType.STATUS_CHANGE` or `INFO` (maps to UI `"info"`, ~8s toast).
- Neither is in `isActionable()` (`web-app/src/lib/notification-policy.ts:29-31`, which is
  only `approval_needed`/`question`), so both are transient toasts that also land in the
  persistent NotificationPanel history — the history is the durable "why did this happen"
  record even after the toast times out.

## 2. Current exposure of `BacklogController`/backlog-enabled state in the UI

No dedicated status indicator exists today. The only UI surface for backlog on/off state is
the **generic feature-flag toggle**, not anything quota-aware:

- `server/services/feature_flag_service.go`: `GetFeatureFlags`/`UpdateFeatureFlag` RPCs, backed
  by `knownFeatureFlags` (static list with `name`+`description`) and an optional
  `FeatureController` per name (`BacklogController` is registered under `"backlog"` at
  `server/dependencies.go:1131`).
- Frontend: `web-app/src/lib/contexts/FeatureFlagsContext.tsx` fetches/holds flag state;
  consumed by `web-app/src/app/settings/features/page.tsx` (a plain on/off toggle row with an
  "On"/"Off" badge — see `FeaturesPage`, lines 50-82) and by `useFeatureFlag("backlog")` in
  `Navigation.tsx` (gates whether the Backlog nav item even renders — unrelated to *why* it's
  currently paused).

Important existing behavior worth reusing: `GetFeatureFlags` already treats the wired
controller as the source of truth over the persisted disk flag
(`feature_flag_service.go:104-107`, "If a controller is wired, its live state is the source
of truth"). That means **if the quota gate calls `BacklogController.Enable()/Disable()`
directly** (as the requirements mandate — reusing `IsEnabled()` as the sole enforcement
point), the existing Settings → Feature Flags page will automatically show "Backlog: Off"
the moment the gate disables it, with zero new frontend code. The gap is that the badge only
says On/Off — it has no field for *why* it's off or *whether it will resume on its own*.

**Recommendation**: don't add a new status page/indicator. Extend the existing
`FeatureFlag` proto message with one new optional string field (e.g. `status_detail`) that
`GetFeatureFlags` populates from the controller when present (analogous to how `IsEnabled()`
is read today) — e.g. `"Paused: session-quota headroom below threshold. Resumes automatically
when headroom recovers."` Render it as a second line under the existing description in
`FeaturesPage` (reuse the existing `flagDescription` style — no new component). This keeps
the "why" visible persistently (unlike the toast, which times out), without a new screen.

## 3. User mental model: what builds trust in an auto-pause

- **Why**: must distinguish "paused for quota" from "paused because I manually turned it
  off" or any other future reason — a bare Off toggle is not enough (see §2). The
  notification title/message should name the reason explicitly (mirrors
  `capacity_monitor.go`'s message pattern, e.g. `"Backlog paused: session-quota headroom
  below threshold (%.0f%% remaining, threshold %.0f%%)"`), and the persistent status_detail
  field should carry the same reason so it's visible after the toast clears.
- **How long**: do not promise an ETA. The requirements note quota headroom is *inferred*,
  not polled from a first-class API — a countdown or predicted-resume-time would be false
  precision. State the mechanism instead of a time: "resumes automatically once headroom
  recovers" (plus, if useful, the check cadence — "re-evaluated every reconcile cycle" —
  without committing to a specific ETA).
- **Can I override**: yes, via the existing Settings → Feature Flags manual toggle — this
  already works today with no new code, since `UpdateFeatureFlag` calls `ctrl.Enable()`
  unconditionally regardless of what auto-disabled it. **Flag a real footgun**: if the user
  manually re-enables while quota is still genuinely low, the next reconcile tick will
  immediately re-disable it, and the toggle will silently flip back to Off — this is the
  same flicker shape already fixed once in this codebase for a different feature (see commit
  `ed0fda703`, "stop plan-approval UI flicker on stuck items"). Recommend either (a) a short
  grace period after a manual override before the quota gate is allowed to re-pause, or at
  minimum (b) making the re-pause post its own visible notification each time (not a silent
  re-flip) so the behavior is legible rather than looking like the toggle didn't take. Given
  the "no silent flip" constraint already in requirements.md, (b) is close to free since the
  pause-notification path is being built anyway — just don't suppress it because "we just
  paused for the same reason 30 seconds ago" (the 5-minute self-rate-limit from §1 should be
  short-circuited specifically for a pause that follows a manual user override, so the user
  gets immediate feedback rather than a 5-minute-silent re-pause).

## 4. Accessibility

No new UI component is being introduced — pause/resume reuses the existing
`NotificationToast`/`NotificationPanel` pipeline (already accessible; not re-audited here)
and the existing `FeaturesPage` toggle row/badge (`aria-label`, `aria-pressed` already present
at `page.tsx:71-72`). The one net-new UI element is a plain text line (`status_detail`) added
to that existing row, which needs no ARIA beyond what the row already has — it's descriptive
text, not an interactive control. No accessibility work is needed beyond that; do not add
generic WCAG boilerplate for a feature that adds zero new interactive surfaces.

## Summary of recommended surface (kept intentionally small)

1. Pause/resume notifications via the existing `session.Notifier`/`EventBusNotifier` →
   `events.NewNotificationEvent` pipeline (no new event/notification infra).
2. One new optional `status_detail` string field on the existing `FeatureFlag` proto,
   surfaced as a second text line on the existing Settings → Feature Flags row for
   `"backlog"` (no new page/component).
3. No countdown/ETA — state the mechanism ("resumes automatically when headroom recovers"),
   not a predicted time.
4. Manual override already works via the existing toggle; the only behavior change needed
   is to not silently re-suppress the very next re-pause notification after a manual
   override, to avoid a silent-flicker regression of the kind already fixed once in this
   codebase (`ed0fda703`).
