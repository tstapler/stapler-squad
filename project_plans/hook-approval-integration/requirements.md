# Requirements: Hook Approval Integration

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-04-07

## Problem Statement

The hook approval workflow in Stapler Squad is functionally correct but experientially broken for the user managing multiple AI sessions. Three distinct failures compound each other:

1. **Approval state is not shared across views.** When a user approves or denies a hook from the notification popup, the review queue does not reflect the resolution. The notification may persist alongside a stale queue entry. There is no single source of truth being broadcast to all views — each view independently polls or displays cached state.

2. **Notifications truncate the command.** The `PermissionRequest` notification shows the `ToolInput` but truncates the command string (e.g., `rtk proxy git show HEAD:.github/...`). The user cannot approve in-place because they can't see what they're approving.

3. **The review queue does not surface pending approvals.** The review queue is intended to be the user's action hub for everything requiring attention. Hook approval requests (`PendingApproval` records in `ApprovalStore`) are not appearing as `Approval` reason entries in the queue. Sessions with pending approvals may only appear as `Stale` instead.

**Who has this problem**: Any developer using Stapler Squad as a session manager who wants to use the review queue as their primary action surface.

## Success Criteria

1. **Approve-once, resolved-everywhere**: Approving or denying a hook request from *any* UI location (notification toast, review queue row, session detail view) immediately removes that approval from all other views. No stale entries remain.

2. **Full command visible**: The notification and review queue both display the complete command/tool input without truncation. User can make an informed decision without opening the session.

3. **Pending approvals in the review queue**: Sessions with pending `PendingApproval` records appear in the review queue with `Reason: Approval` (not just `Reason: Stale`). Approve/deny actions are available inline.

4. **Review queue is the approval hub**: A user who only looks at the review queue can see and resolve all pending approvals across all sessions.

## Scope

### Must Have (MoSCoW)
- Pending `PendingApproval` records from `ApprovalStore` surface as queue items with reason `Approval`
- Approve/deny actions available inline in the review queue
- Full `ToolInput` (no truncation) displayed in notification toasts and review queue items
- Resolving an approval in any view (notification OR queue) clears it from all other views
- Approval state changes push real-time updates via the existing `EventBus` / SSE streaming

### Should Have
- Review queue shows approval age and tool name for each pending request
- Multiple pending approvals for the same session are shown as separate actionable items
- Orphaned approvals (loaded from disk after restart) are visually distinguished

### Out of Scope
- New notification delivery mechanisms (email, Slack, etc.)
- Rewriting the approval persistence layer
- Changes to the Claude Code hook protocol
- Any changes to `approval_handler.go`'s HTTP blocking behavior

## Constraints

- **Tech stack**: Go backend (ConnectRPC), React/Next.js frontend — no new dependencies
- **Existing infrastructure**: Build on `ApprovalStore`, `ApprovalHandler`, `EventBus` in `server/services/`
- **Backward compatibility**: Existing sessions, rules, and approval flows must continue to work
- **No new external deps**: Prefer solutions using existing libraries and patterns

## Context

### Existing Work

The approval infrastructure is substantially complete:
- `ApprovalStore` (`server/services/approval_store.go`) — thread-safe in-memory + disk-persisted store for `PendingApproval` records, indexed by session ID, with `ListAll()` and `GetBySession()` methods
- `ApprovalHandler` (`server/services/approval_handler.go`) — HTTP hook handler that blocks waiting for user decision via `decisionCh`, publishes events to `EventBus` on creation/resolution
- `ReviewQueueChecker` interface already exists in `approval_handler.go` — designed to trigger immediate review queue checks on new approval creation
- `approvalNotificationStamper` interface exists — for stamping approval outcomes on notification records after resolution

The missing pieces are wiring:
1. The review queue query logic does not call `ApprovalStore.ListAll()` to populate `Approval` reason entries
2. Notification toasts truncate `ToolInput` command strings at display layer
3. No broadcast on approval resolution to invalidate/dismiss notifications in other views

### Stakeholders

- **Primary**: Tyler (solo developer) using the web UI as their action hub
- **General**: All Stapler Squad users who use the review queue feature

## Research Dimensions Needed

- [ ] **Stack** — How does the existing review queue currently query and score sessions? What event types does `EventBus` publish on approval events?
- [ ] **Features** — How do other session managers (e.g., Tmuxinator, workflow tools) handle cross-view state sync for action items?
- [ ] **Architecture** — What is the right pattern for broadcasting approval state changes — push via SSE/EventBus vs pull via polling? How does the frontend currently receive review queue updates?
- [ ] **Pitfalls** — Race conditions between approval resolution and notification dismissal; orphaned approvals after server restart; multiple concurrent approvals for the same session
