# Research: UX — pr-fix-steering

**Dimension**: UX | **Phase**: 2 — Research

Scope note: this is a backend reconciliation-loop change with exactly two user-visible
surfaces — the steer message text itself (echoed in the target session's terminal) and a
backlog-item notification/comment recording the attempt. There is no new screen or
component. Findings below are scoped to those two surfaces only.

## 1. Existing stylistic precedent

Three things already exist and should be reused verbatim, not reinvented:

**a. The problem-description content itself: `PRStatus.render()`**
(`session/git/worktree_git.go:568-621`) already builds the exact "what's wrong with this
PR" text as Markdown with `##`-level sections, in a fixed order — conflict first, then CI,
then reviews, then comments:

```
## Merge conflict
This PR's branch has merge conflicts against its base branch (mergeStateStatus=...)
...
## Failing CI checks
- build FAILED
## Review: changes requested by @author
<review body>
## Reviewer comments
@author: <comment body>
```

This is stored as `PRStatus.FeedbackText` and passed as `fixContext` into
`AutoReopenForPRFix` (`server/services/backlog_service_triage.go:2018`, called from
`session/backlog_lifecycle_review.go`/`backlog_lifecycle.go`) when spawning a **new** fix
session — `AutoReopenForPRFix` wraps it once more as `[PR Fix context - PR #%d (%s)]\n%s`
(`backlog_service_triage.go:2100`) before prepending it to the item's notes. **The new
steer message should reuse `FeedbackText` as its body**, not re-derive its own summary of
`CIFailing`/`HasBlockingReviews`/`HasConflicts` — the formatting, ordering, and even the
conflict section's rebase instructions ("push with `--force-with-lease`", "run `git diff
--stat`...") already exist and are already tuned by a real incident. Reusing it also means
a human reading the terminal sees the *same* wording whether the item spawned a fresh
session or steered a live one — one voice, not two.

**b. The notification format: `notifySteerSent`**
(`server/services/session_service.go:3029-3042`) is the two-thing pattern to follow for
every steer notification in this codebase:
- Title: short, present-tense, human ("Steering input sent")
- Message: `"<session title>: <message body>"` — session identity first, content second
- `NotificationType_INFO` / `NotificationPriority_MEDIUM`
- `nil` metadata (no `item_id`) — because a manually-triggered steer via `UpdateSession`
  isn't necessarily backlog-linked

**c. The blocked-respawn audit trail: `notifyRespawnBlockedByActiveSession`**
(`server/services/backlog_service_triage.go:1363-1394`) is the specific precedent this
project extends, per its own doc comment ("closes the audit-trail gap ... zero
operator-visible signal"). Its shape:
- `MarkStuck(ctx, itemID, <StuckReason>, ...)` + `MarkStuckNotified` → surfaces as a
  `BlockerChip` (`web-app/src/components/backlog/BlockerChip.tsx`) in the item's always-
  visible `LifecycleSummary` header (icon + label + "how long" duration) — this is the
  *inline, on-the-item* half of the audit trail.
- `eventBus.Publish(events.NewNotificationEvent(...))` with `NotificationType_INFO` /
  `NotificationPriority_LOW`, title `"Auto-respawn skipped — session already active"`,
  message `"<item title> — automatic respawn (<caller>) was skipped because ...: <detail>"`,
  and **`metadata: {"item_id": itemID}`** — this is what makes `NotificationItem.tsx`
  (`web-app/src/components/ui/NotificationItem.tsx:257-266`) render a "View in Backlog"
  link. This is the *global notification bell* half.

The new steer-record should follow (c)'s two-part shape (BlockerChip/stuck-row +
`item_id`-tagged notification), not (b)'s bare `notifySteerSent` shape — because this is an
*automated, backlog-item-scoped* action, and the existing `respawn_blocked_active` pattern
is exactly the "self-heal action needs a visible, item-linked record" precedent the
project's standing preference (`feedback_document_ai_decisions_in_edge_cases`) already
established for this family of code paths. Always include `item_id` in metadata so it's
one click from the notification to the item, matching every other backlog-originated
notification in this codebase.

## 2. JTBD: what should the content include

Operator's two jobs, in priority order:
1. **Trust, without looking**: "the pipeline saw the new CI failure/review/conflict and
   told the agent" — satisfied by the notification existing at all, low-effort to scan.
2. **Fast manual takeover if it didn't work**: satisfied only if the record names the
   *specific* reason (not "a problem was detected") and — for Claude Code sessions — the
   remediation path, so Tyler can paste `/github:pr-ship` himself without re-deriving what
   broke.

Content that satisfies both without being verbose:
- **Notification title**: name the PR and the reason category, e.g. `"Steered active
  session — PR #157 has failing CI"` (mirrors `notifyRespawnBlockedByActiveSession`'s title
  pattern of naming the condition, not just "something happened"). If multiple reasons are
  true simultaneously (CI failing *and* conflicts), name the set, not just the first one —
  a truncated title that only mentions CI when conflicts are also present would send Tyler
  down the wrong manual path if the steer itself failed.
- **Notification body**: item title, which session was steered (title/UUID — matching
  `notifySteerSent`'s "session identity first" convention), and a short reason summary —
  not the full `FeedbackText` Markdown blob (that's the terminal's job, per §1a; duplicating
  a multi-paragraph CI/review dump into the notification card makes the card unreadable in
  the compact `NotificationPanel` dropdown, which is not designed for long-form content —
  see `NotificationItem.tsx`'s `itemMessage` styling, a single `<p>`).
- **Terminal/steer message**: the full `FeedbackText`, per §1a — the live session is where
  the operator (or the agent itself) needs the complete detail to act on, not the
  notification card.

## 3. Error states

Two distinct failure surfaces, and they should read differently because they imply
different next actions:

| Outcome | Terminal (agent's PTY) | Notification/BlockerChip |
|---|---|---|
| Steer delivered successfully | Full `FeedbackText` appears as injected input (existing `SendKeys`/`SendCommandImmediate` behavior — unchanged) | INFO/LOW notification, `item_id`-tagged: "Steered active session — <reason>". Resolves any prior open `respawn_blocked_active`-family stuck row for this item (same `resolveRespawnBlockedActiveLogged` pattern, `backlog_service_triage.go:1400-1409`), since the condition it recorded (session blocked, now unblocked-by-steer) has cleared. |
| Steer attempt itself fails (`SendKeys` timeout, session died between detection and steer, message too long) | Nothing — by definition, the message never reached the session. This is the important asymmetry: a failed steer produces **zero terminal signal**, so the notification is the *only* record. | Must NOT reuse the success notification's wording. Should read as a distinct, actionable failure — e.g. "Auto-steer failed — PR #157 needs attention, could not notify the running session (<error>)" — and should escalate to a **stuck row** (WARNING/MEDIUM or higher priority than the success case's LOW), since "detected a problem, tried to tell the agent, failed" is strictly worse than the pre-existing `respawn_blocked_active` state it's replacing (that state at least says "look at this"; a silently-failed steer says nothing unless this notification exists). This is precisely the gap `notifyRespawnBlockedByActiveSession`'s own doc comment was written to close — a failed steer must not regress back to "zero operator-visible signal." |
| Message too long (`session.MaxSteerMessageLength` = 10,000 bytes, `session/instance.go:139`) | N/A — never sent | Same failure-shape as above, but the message text should say so plainly ("fix context exceeded the steer size limit") rather than a generic "failed" — this is a knowable, pre-send condition (the caller can check length before calling `SendKeys`/`SendCommandImmediate`), not a runtime fluke, so the operator should be able to tell the two apart at a glance rather than treating every failure as "try again." |

Do not collapse success/failure into one "steer attempted" notification with a status field
buried in the body — `NotificationType`/`NotificationPriority` are the fields this codebase
already uses to make severity scannable at the icon/color level in `NotificationPanel`
(`notificationTypeIcon`/`priorityColor` in `notificationMapping.ts`), and a failed
auto-steer is the one outcome here that most needs to stand out from routine INFO noise.

## 4. Existing activity/history surface — where this fits

Two candidate timelines already exist on a backlog item; the steer record belongs in the
notification/stuck-row system, not the third one:

1. **Global notification bell** (`NotificationPanel`/`NotificationItem`,
   `NotificationsPage`) — every `eventBus.Publish(events.NewNotificationEvent(...))` call
   lands here, including `notifyRespawnBlockedByActiveSession`'s existing respawn-blocked
   notices and `notifySteerSent`'s manual-steer notices. **This is where the new steer
   notification goes** — same publish call the sibling functions already make, tagged with
   `item_id` for the "View in Backlog" link.
2. **`LifecycleSummary`'s `BlockerChip` row** — driven by `StuckBacklogItem` rows
   (`MarkStuck`/`ResolveStuck`), shown inline on the item's always-visible header
   (`web-app/src/components/backlog/detail/LifecycleSummary.tsx`,
   `BlockerChip.tsx`). This is where a **failed** steer should also register (as a stuck
   reason), so it's visible without opening the notification bell at all — consistent with
   how `respawn_blocked_active` already works today. A **successful** steer should not add
   a persistent chip (nothing is "stuck" — the point of steering was to avoid that state),
   and should resolve any stuck row it's replacing.
3. **`ActivityLogSection`** (`web-app/src/components/backlog/detail/ActivityLogSection.tsx`)
   — this is a *different* mechanism: free-form notes posted via the `post_backlog_update`
   MCP tool, authored by an agent session narrating its own work
   (`backlog-item-activity-log` project, ADR-001). It is not fed by `eventBus` notifications
   at all. **Do not post the steer record here** — it would require a new write path
   (`post_backlog_update`-shaped) parallel to the notification system this project's own
   requirements already name as the reuse target ("extending the existing
   `notifyRespawnBlockedByActiveSession` audit trail"), and it's the wrong semantic fit: this
   log is "what an agent did," not "what the reconciliation loop did to an agent."

No new timeline, tab, or component is needed. The existing two-surface pattern (bell
notification + BlockerChip-driven stuck row) already covers both the "trust it happened"
and "something's wrong, here's why" jobs from §2.
