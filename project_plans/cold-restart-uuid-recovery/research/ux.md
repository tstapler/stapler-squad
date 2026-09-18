# UX Research: User-Visible Signal for Cold-Restart UUID Recovery (AC3, stretch)

Agent 5 (UX) — SDD research phase, `cold-restart-uuid-recovery`.

## Question

AC3 in `requirements.md` (lines 87–94, 109–110) makes full UI surfacing of "started
fresh despite an apparent prior conversation" a **stretch goal**, not a hard AC. This
doc scopes what — if anything — is justified for v1.

## 1. Does any existing UI already reflect Claude conversation/resume state?

Yes, but only as raw debug fields, not a status signal.

- `web-app/src/components/sessions/SessionDetailView.tsx:1061-1069` renders
  `session.claudeSession.sessionId` (the conversation UUID) as a monospace value with
  a copy button, labeled "Claude Conversation UUID:".
- `SessionDetailView.tsx:1076-1081` renders `session.historyFilePath` the same way,
  labeled "History File:".

Both are conditionally rendered (`{session.historyFilePath && (...)}`) inside what
reads as a technical/debug info panel — there is no badge, icon, tooltip, or copy
that distinguishes "resumed an existing conversation" from "started fresh." A user
would have to know to look at these two raw values and reason about them manually;
there's no existing plumbing that computes or displays a resumed-vs-fresh boolean.

`SessionList.tsx:822`'s `"...resumed"` feedback string is unrelated — it's
session-lifecycle resume (pause/resume of the tmux session), not Claude conversation
resume.

**Conclusion: no existing UI signal for this specific condition. Would need to be
added from scratch if pursued.**

## 2. Are the backing proto fields actually populated today?

Yes, both are wired end-to-end already:

- `proto/session/v1/types.proto:141-150` defines `history_file_path` (field 41) and
  `claude_conversation_uuid` (field 42) on the session message, with doc comments
  confirming intended use ("Used to pass `--resume <uuid>` when reattaching after
  server restart").
- `server/adapters/instance_adapter.go:106-117` populates the response:
  - `protoSession.ClaudeSession = &sessionv1.ClaudeSession{SessionId: cs.ConversationUUID, ...}` (from `inst.GetClaudeSession()`)
  - `protoSession.HistoryFilePath = snap.HistoryFilePath`
- `session/instance_claude.go:361,466,479` and `session/instance.go` (multiple
  clear-sites) show `HistoryFilePath`/conversation UUID are actively maintained on
  `Instance` through the session lifecycle (set on link, cleared on
  `ClearConversationState`, etc.).

So the raw UUID/path data is not the gap — it's already exposed via
`SessionDetailView`. **What's missing is a derived signal**: a flag/enum saying
*why* the session started the way it did (resumed cleanly / started fresh, no prior
conversation / started fresh despite an apparent prior conversation existing). No
such field exists on the proto or `Instance` today — adding one would be new
plumbing (backend field + adapter wiring + a UI element to read it), which is exactly
the stretch-goal scope requirements.md flags for planning to size separately.

## 3. Minimal viable signal: does an existing notification mechanism fit?

Yes — and it's a strong fit, with a direct precedent already in the codebase.

`server/notifications/` (`store.go`, `subscriber.go`) plus
`events.NewNotificationEvent(...)` is a general-purpose, **backend-driven**
notification pipeline already used from plain session-lifecycle Go code (not just
Claude-Code-hook JSON payloads). It flows through to the existing
`NotificationToast`/`NotificationPanel`/`NotificationContext` UI
(`web-app/src/components/ui/NotificationToast.tsx`,
`web-app/src/components/ui/NotificationPanel.tsx`) with zero new frontend component
work required — those components already render arbitrary
`title`/`message`/`priority`/`notificationType` payloads keyed by `sessionId`.

The proto already defines a `NOTIFICATION_TYPE_WARNING` (value 8,
`proto/session/v1/types.proto:783`) and `NOTIFICATION_TYPE_FAILURE` (value 9), both
described as "Errors and Warnings (High/Urgent Priority by default)" — semantically
exactly what "conversation could not be resumed, started fresh" is.

**Direct precedent to copy**: `server/services/session_service.go:3908-3933`
(`onRateLimitDetected`) and `:3935-3961` (`onRateLimitRecovery`) show the exact
call shape for "backend detects an anomalous session-lifecycle condition → publish a
notification tied to that session":

```go
notifID := fmt.Sprintf("rl-detect-%s", sessionID)
s.eventBus.Publish(events.NewNotificationEvent(
    sessionID, inst.Title, notifID,
    int32(8), // NotificationType_WARNING
    int32(3), // NotificationPriority_HIGH
    title,
    fmt.Sprintf("Session hit the usage limit%s.", resetMsg),
    events.SessionScopedMetadata(nil, linkedItemID),
))
```

A `cold-restart-fresh-%s` notification with `NotificationType_WARNING` and a message
like `"Session %q started fresh — a prior conversation may exist but could not be
resumed"` fits this pattern with no new UI code: it would surface in the existing
toast + notification panel automatically, the same way rate-limit warnings do today.

## 4. Recommendation

**For v1: no new UI component. Reuse the existing notification pipeline, gated to
only the "ambiguous" case.**

- The structured WARN log (already a hard AC, item 3) plus a single
  `events.NewNotificationEvent(..., NotificationType_WARNING, ...)` call at the same
  call site is sufficient. This should fire **only** for the "started fresh despite
  an apparent prior conversation" branch — not for genuinely-first-time sessions —
  to avoid noise (mirrors how `onRateLimitDetected` only fires on the anomalous
  condition, not on every session tick).
- This costs roughly one function + one call site in
  `server/services/session_service.go` (or wherever the revival/fallback decision
  lands per the plan), following the `onRateLimitDetected`/`onRateLimitRecovery`
  pattern exactly. No proto changes, no new frontend component, no new registry
  entries beyond whatever the feature registry rule already requires for touching
  `session_service.go`.
- **Defer to an explicit follow-up**: a persistent session-card badge/indicator
  (e.g. "conversation not recoverable" chip) driven by a new derived proto field.
  That requires proto schema changes, adapter wiring, and new frontend rendering —
  real feature work, correctly out of scope for a reliability-fix stretch goal. Note
  it in the plan as a named follow-up rather than silently dropping it.
- Toasts are transient by design (`toastAutoCloseMs`/`toastAutoMinimizeMs` in
  `web-app/src/lib/notification-policy.ts`), but `NotificationPanel.tsx` persists a
  history the user can review later — so even a fire-and-forget toast isn't lost if
  the user misses the pop-up, satisfying "user-visible" without a permanent UI
  element.

## 5. Accessibility

N/A for this scope. No new UI component is recommended for v1 — the notification is
rendered by the existing `NotificationToast`/`NotificationPanel` components, whose
accessibility characteristics (if any gaps exist) are pre-existing and orthogonal to
this fix. If the deferred session-card badge follow-up is later built, it should get
its own accessibility pass at that time (icon needs an accessible label/`aria-label`,
not color-only signaling) rather than speculative guidance now.
