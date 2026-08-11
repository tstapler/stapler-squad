# Feature Research: review-session-notification-cleanup (Agent 2)

## Open Question Resolution

**The requirements.md open question is resolved: there is no separate, dedicated
code path that produces a TASK_COMPLETE/Idle/Stale-style notification for a
genuinely headless (no-`Instance`) triage or re-review session — because that
exact wording (`"Task Complete"`, `"Session idle - ready for next task"`,
`"No activity for Nh Nm..."`) is only ever constructed in
`session/review_queue_determiner.go` (`AttentionReason.String()` at
`session/queue/queue.go:30-49`, esp. `ReasonTaskComplete → "Task Complete"` at
`:42-43`), consumed exclusively by `session/review_queue_poller.go`'s
`checkSession`/`Determine()`, which only ever iterates `rqp.instances
[]*Instance` — a list populated *solely* by `AddInstance`/`SetInstances` calls
that only ever fire from real, tmux-backed session creation
(`server/services/session_service.go:870-875`, `:919-924`, `:1493-1502`,
`:3165-3171`; also `server/dependencies.go:753-763` at startup reconciliation).**

**`TriggerTriage` (`server/services/backlog_service_triage.go:1781-1798`) and
`TriggerReReview`'s headless branch (`:2109-2327`) never call `AddInstance` or
`reviewQueuePoller.AddInstance` for their synthetic
`headless-triage-<uuid>`/`headless-re-review-<uuid>` `ItemSession.SessionUUID`
values — no `session.Instance` is ever constructed for them at all. They are
therefore structurally invisible to `shouldSkipSession`/`Determine`/`OnItemAdded`
by construction, not because a guard happens to catch them.** Confirmed by
grepping every `AddInstance`/`SetInstances` call site in `server/` and
`session/` (none is reachable from `backlog_service_triage.go`'s headless-pool
code paths) and by reading `TriggerTriage`'s async goroutine
(`:1805-1900`+) end-to-end: on success or failure it only calls
`s.storage.UpdateItemSessionEnded` / persistence methods and, on a specific
persistence failure, `notifyTriagePersistFailure` (`:229-246`) — a distinct,
already-item_id-tagged notification, never the generic Idle/Stale copy.

**What *does* produce the exact reported notifications for backlog-linked
sessions today is the one path requirements.md already documented — a real,
`Hidden=true` `session.Instance` reaching the poller — but there are two call
sites that create such an instance, not one:**

1. `server/services/session_service.go:826-833` `SpawnReviewSession` (title
   `"review:" + item.ID[:8]`), called from `session/review_gate.go:341`
   (`spawnReviewGate`, the "real, hidden, tagged review session.Instance" —
   its own comment literally says this is deliberate so the review
   "participates in the same visibility/attention mechanism ... as every
   other session").
2. `server/services/backlog_service_triage.go:2351-2359` — `TriggerReReview`'s
   **non-headless fallback branch** (used when `s.headlessPool == nil`, or
   really whenever `s.sessionCreator` is used instead of the headless path):
   `CreateDirectorySession(ctx, title, item.RepoPath, reReviewPrompt,
   []string{"backlog:review"}, !useAutonomous, true /*hidden*/)` — also
   `Hidden=true`, `SessionRole=Review`, and (when `s.autonomousStarter != nil`)
   driven by `AutonomousDriver` via `StartAutonomousDriverWithTimeout`
   (`:2357-2358`).

Both are real `*Instance`s, both get `AddInstance`'d
(`session_service.go:870-875`/`919-924`/`1493-1502`), both are `Hidden=true`,
and both therefore flow into `rqp.instances` → `Determine()` → `OnItemAdded`
exactly as requirements.md's existing analysis describes. **A single fix at
`Determine()`/`checkSession`/`OnItemAdded` gated on `Instance.Hidden` (or the
`ItemSession.SessionRole` looked up via `session_uuid`) covers both call
sites** — no second mechanism needs separate suppression logic. The
`backlog_service_triage.go:1781`/headless-triage-uuid code path requirements.md
flagged as the "third, parallel" candidate does **not** need suppression logic
added to it at all, because it never reaches the notification system via this
route in the first place — see "Additional finding" below for the one
notification path it *does* reach that still needs a metadata/gating fix, just
not for the reason originally suspected.

## Additional finding: a second, previously-undocumented generic-completion notifier — `autonomous_orchestration_service.go`

`server/services/autonomous_orchestration_service.go`'s `onAutonomousDriverComplete`
(`:228-546`) is a **second, independent generic-completion notifier**, not
discussed in requirements.md at all. It fires whenever *any* `AutonomousDriver`-run
session (real `Instance`, started via `StartAutonomousDriverForInstance` /
`StartAutonomousDriverWithTimeout`) finishes, and does **not check
`Instance.Hidden` anywhere in the function**:

- `SessionRoleReview` (`:428-479`) is explicitly exempted — both the `outcome.Done`
  and stuck sub-branches `return` before reaching the generic notifier
  (comment at `:429-430`: "no generic notification (that would duplicate the
  review-specific one)"). **This is the one role that's already correctly
  handled** for the two Hidden-Instance-driven paths above (when
  `TriggerReReview`'s autonomous fallback finishes, it exits through this
  branch and produces no duplicate notification).
- `SessionRoleTriage` (`:306-324`): the *stuck* (`!outcome.Done`) sub-branch
  publishes its own `"Triage did not complete"` notification and `return`s
  (`:310-321`) — skips the generic notifier. **The *done* sub-branch does
  not `return`** (`:323-324`, just sets `toStatus` and falls through) — if
  this ever ran to completion, it would fall through to the generic
  `"Autonomous fix complete"` notifier at `:518-545` with **no** `Hidden` or
  role check. **Currently dead in practice**: no live code path attaches a
  `SessionRoleTriage` `ItemSession` to a real, `AutonomousDriver`-run
  `Instance` — `TriggerTriage` (the only place `SessionRoleTriage` is
  created outside test/seed helpers, `backlog_service_triage.go:1789`) always
  uses the synthetic no-Instance `headlessPool.CallBlocking` path, never
  `StartAutonomousDriverForInstance`. Grep confirms the only other
  `SessionRoleTriage` write site is `backlog_debug_seed_handler.go:299`
  (a debug/test seed helper). Flag this as a **latent gap**: if a future
  change ever routes triage through `AutonomousDriver`, this branch will leak
  an ungated, unmetadata'd notification the instant it's wired up.
- `SessionRoleWork` (`:325-427`): correctly falls through to the generic
  notifier — in scope per requirements.md ("real, non-headless interactive
  sessions ... should keep generating ... notifications exactly as today"),
  and these instances are *not* Hidden (spawned via
  `CreateWorktreeSession`/`CreateDirectorySession(..., false, false)` at
  `backlog_service_triage.go:702-708`), so no suppression is wanted here.
- `default` (unrecognized role, `:480-487`): falls through to the generic
  notifier by design (a defensive catch-all so a newly-added role isn't
  silently dropped) — worth a code comment cross-reference once AC1's fix
  lands, so a future new role doesn't have to rediscover this fallthrough.

**Concrete AC2 gap found here, independent of the open question**: both
generic-notifier call sites in this file pass **`nil` metadata**:
`:318` (`SessionRoleTriage` stuck) and `:544` (the generic done/stuck
notifier) — even though `item.ID` is in scope at both call sites (looked up
via `GetItemSessionBySessionUUID` → `GetBacklogItem` a few lines above each).
Contrast with every notification in `backlog_service_triage.go`
(`notifyReworkCapHit`, `notifyRepeatedFailure`,
`notifySpawnAndRollbackFailed`, `notifyTriagePersistFailure`,
`notifyIfActiveWorkSessionStale`, and the two `TriggerReReview` blocked-verdict
notifications), which all correctly set
`map[string]string{"item_id": itemID}` as the last argument. This file should
follow that same pattern. (The generic notifier at `:540-545` uses
`sessionUUID` as the event's `sessionID` positional arg, not `item.ID` — so
even the "sessionID happens to equal item_id" fallback some other call sites
rely on doesn't apply here.)

## Full call-site catalog: `events.NewNotificationEvent(` / `eventBus.Publish(`

Grepped every call site across `server/` and `session/` (`grep -rn
"events.NewNotificationEvent("`). Classified against "fires for
headless/hidden/review/triage sessions?" and "already guards on
Hidden/SessionRole/OneShot?":

| Call site | Fires for headless/review/triage? | Guards Hidden/Role today? | Notes |
|---|---|---|---|
| `server/review_queue_manager.go:355` (`OnItemAdded`) | **Yes** — the primary documented path (AC1's main target) | No (only excludes `ReasonApprovalPending`, `:337`) | Fed exclusively by `rqp.instances`; both real-Instance review paths above land here |
| `server/services/backlog_service_triage.go:145` (`notifyReworkCapHit`) | No — fires on rework-cap hit for a *work* session context | N/A | Already `item_id`-tagged |
| `server/services/backlog_service_triage.go:180` (`notifyRepeatedFailure`) | No | N/A | Already `item_id`-tagged |
| `server/services/backlog_service_triage.go:214` (`notifySpawnAndRollbackFailed`) | No | N/A | Already `item_id`-tagged |
| `server/services/backlog_service_triage.go:239` (`notifyTriagePersistFailure`) | Only touches triage indirectly (persistence failure after a headless triage call) — not a generic completion/idle notice | N/A (distinct, specific title/body) | Already `item_id`-tagged |
| `server/services/backlog_service_triage.go:987` (`notifyIfActiveWorkSessionStale`) | No — explicitly gated to `SessionRoleWork` (`hasActiveWorkSession`, `:884-890`); this is the already-shipped `review-gate-stale-session-rework` project's mechanism, unrelated to review/triage roles | Filters to `Role == SessionRoleWork` already | Already `item_id`-tagged; out of scope here |
| `server/services/backlog_service_triage.go:2041` (branch-drift-blocked re-review) | Touches a re-review flow, but is a specific error notice, not generic Idle/Complete | N/A | Already `item_id`-tagged |
| `server/services/backlog_service_triage.go:2132` (codebase-dir-missing re-review) | Same as above | N/A | Already `item_id`-tagged |
| `server/services/autonomous_orchestration_service.go:310` (Triage stuck) | **Yes, in principle** — `SessionRoleTriage`, but only reachable if a real `AutonomousDriver` Instance ever gets that role (currently dead, see above) | Partial — only fires on stuck triage, never on done | **metadata is `nil`** — AC2 gap if/when this branch becomes live |
| `server/services/autonomous_orchestration_service.go:364` (auto-rework paused) | No — `SessionRoleWork` turn-cap-parked notice | N/A | metadata `nil` too, but session is a visible work session (lower priority to fix) |
| `server/services/autonomous_orchestration_service.go:540` (generic done/stuck) | **Yes** — reachable for any completed `AutonomousDriver` Instance whose role isn't `Review` and isn't stuck-`Triage`; currently only live for `SessionRoleWork` and `default` | **No `Hidden` check at all** | **metadata is `nil`** — concrete AC2 gap; see "Additional finding" above |
| `server/services/capacity_monitor.go:288` | No — capacity/backpressure notice, unrelated to session roles | N/A | Out of scope |
| `server/services/notification_service.go:107` | Generic `SendNotification` RPC passthrough (user/API-initiated), not a session-lifecycle detector | N/A | Out of scope |
| `server/services/session_service.go:1763` | Session-creation-progress-failure notice (spawn error), not idle/complete | N/A | Out of scope |
| `server/services/session_service.go:3864`, `:3889` | Rate-limit detection notices | N/A | Out of scope |
| `server/services/approval_handler.go:456`, `:482` | Approval-flow notices (already deduped against `OnItemAdded`'s `ReasonApprovalPending` skip per its own comment) | N/A | Out of scope |
| `server/mcp/tools_backlog.go:873` | MCP-tool-initiated notice (e.g. explicit operator action) | N/A | Out of scope |
| `server/server.go:240`, `:286` | Startup/global notices | N/A | Out of scope |

**Net: only two call sites are actually in scope for AC1/AC2 fixes:**
`server/review_queue_manager.go:355` (`OnItemAdded` — the documented primary
target, needs a `Hidden`/`SessionRole` gate before firing, plus `item_id`
metadata for the not-suppressed case) and
`server/services/autonomous_orchestration_service.go:540` (plus, defensively,
`:310` in case the dead `SessionRoleTriage`-done branch is ever activated) —
needs an `Instance.Hidden` (or role) check added, and needs `item_id` metadata
threaded through in all cases, not just the suppressed ones.

## Headless pool itself has no notification/staleness mechanism (research task item 3)

`session/headless/*.go` (`caller.go`, `pool.go`, `runner.go`, `client.go`,
`capability_check.go`, `features.go`) contains **zero** `eventBus`/
`NewNotificationEvent` references (confirmed by grep) and no independent
timeout/staleness *detector* — only context-timeout constants
(`DefaultCallTimeout` 900s, `MaxCallTimeout` 1800s,
`CodebaseReadCallTimeout` 600s, `capabilityCheckTimeout` 30s, all in
`features.go`/`capability_check.go`) that simply make `CallBlocking` return an
error when exceeded. Every caller (`TriggerTriage`, `TriggerReReview`) handles
that error itself via logging + `UpdateItemSessionEnded`, with specific,
already-`item_id`-tagged notifications only for the few conditions that
warrant one (branch drift, missing codebase dir, persistence failure) — there
is no generic "headless call timed out" notification today, so nothing there
needs suppressing.

## Existing suppression / dedup patterns to follow for consistency (research task item 4)

1. **Reason-based dedup in `OnItemAdded`** (`server/review_queue_manager.go:337`):
   `if rqm.eventBus != nil && item.Reason != session.ReasonApprovalPending` —
   the existing precedent for "skip publishing a notification for this
   specific case, but still do the other side effects (queue broadcast,
   auto-PR check)". AC1's fix should follow this exact shape: wrap the
   existing `eventBus.Publish` block in an additional condition, not touch
   `rqm.publishToClients(event)` above it (the live review-queue UI still
   needs the item for its own internal bookkeeping/queue state, since
   `shouldSkipSession` already keeps Hidden sessions out of the *queue*
   itself — the notification-specific gate is genuinely orthogonal and
   additive, matching AC1's explicit "independent of the poller's existing
   shouldSkipSession queue-visibility guard" scope note).
2. **`item_id` metadata convention**: every well-behaved notification in
   `backlog_service_triage.go` uses `map[string]string{"item_id": itemID}` —
   this is the literal shape AC2 wants generalized to `OnItemAdded` (which
   needs an `ItemSession.session_uuid → backlog_item` lookup, since
   `ReviewItem` doesn't carry `item_id` directly today per requirements.md's
   own analysis) and to `autonomous_orchestration_service.go:540`/`:310`
   (which already *has* `item.ID` in scope and just needs to pass it through
   instead of `nil`).
3. **Role-based filtering precedent**: `hasActiveWorkSession`
   (`backlog_service_triage.go:884-890`, `Role == SessionRoleWork`) and the
   `SessionRoleReview` early-return in `onAutonomousDriverComplete`
   (`:428-479`) are both existing, in-repo examples of "gate behavior on
   `ItemSession.Role`" — the same pattern AC1 wants applied to
   `Determine()`/`OnItemAdded`/the generic autonomous notifier for
   `review`/`triage` roles.
4. **Age-based tombstoning precedent for headless sessions specifically**:
   `server/services/backlog_service_query.go:66-79` (`GetBacklogItem`)
   already tombstones stale `headless-triage-*` `ItemSession` rows
   (`EndedAt == nil`, `time.Since(CreatedAt) > maxTriageSessionAge`) purely
   so the UI doesn't show them as perpetually "running." This is a useful
   precedent for AC3's "notification whose session no longer exists" pruning
   — for headless sessions, "no longer exists" should probably be defined the
   same way (`ItemSession.EndedAt != nil`, or age past this same threshold),
   not by looking for a `session.Instance` (which never existed for these).

## Edge cases (research task item 5)

- **Session starts headless/hidden, later manually "claimed"/un-hidden.**
  Only applies to the two real-`Instance` paths (`SpawnReviewSession`,
  `TriggerReReview`'s autonomous fallback) — a headless-pool triage/re-review
  call has no `Instance` to un-hide in the first place. Need to confirm
  whether any UI/RPC path can flip `Instance.Hidden` back to `false` after
  creation (a quick grep for `.Hidden = false` assignments outside
  construction would resolve this in Phase 3) — if such a path exists, the
  suppression check must read `Hidden` live at publish time (which
  `Determine()`/`OnItemAdded` already do, since they run per-poll-tick), not
  cache a stale value.
- **Review verdict already returned before staleness/idle detection would
  fire.** For the two real-Instance review paths, once
  `submit_review_verdict` is called the session typically exits promptly and
  `handleReviewSessionExited` (`session/backlog_lifecycle.go`, referenced in
  `review_gate.go:321-322`) takes over — but if the poller's next tick runs
  in the narrow window between verdict-submitted and process-exit, `Determine()`
  could still classify it Idle/Stale. Suppressing purely on `Hidden`/`Role`
  (not on verdict-presence) sidesteps this entirely — the fix should not try
  to special-case "verdict already in" as a *different* condition from
  "review-role session," since the role-based suppression already covers it
  regardless of verdict timing.
- **Notification already read/acted on, then pruned by AC3's new
  session-existence check.** AC3 must only prune notifications for
  sessions/instances that no longer exist, not filter by read/unread state —
  confirm `server/notifications/store.go`'s existing notification struct
  distinguishes "acknowledged/read" from "referenced session gone" before
  adding the new prune condition, so a notification the user already saw and
  dismissed isn't conflated with one that's stale-and-should-be-removed only
  because of dangling-reference cleanup (these are two independent axes: one
  user-driven, one integrity-driven). This is a Phase 3 design detail, not
  fully resolved here — flagging for the plan phase to confirm
  `store.go`'s schema before designing the prune query.
