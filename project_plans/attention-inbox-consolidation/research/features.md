# Research: Features (industry survey, codebase inventory, prior decisions, edge cases)

Date: 2026-10-23. Confidence labels: VERIFIED = source opened/quoted via search result or file read in this repo; INFERRED = my synthesis; UNVERIFIED = search did not confirm.

## 1. Industry survey

| Product | "Needs my input" | "Finished" | "Gone quiet / stale" | Timeline | Durable vs transient | Done / snooze / dismiss |
|---|---|---|---|---|---|---|
| GitHub notifications | `reason:` filters (review-requested, mentioned, assigned) split "participating" from "watching" | Read/Unread only; no completion concept | None (no stale view) | Per-thread page, not in inbox | Inbox is durable: unsaved kept 5 months, Saved kept indefinitely | Done (removes from inbox, queryable `is:done`), Save (pin), read/unread toggle, bulk triage, up to 15 custom filters. No snooze. |
| Linear inbox | All activity lands in inbox; cannot customize what enters it | Read state | None | Issue activity feed | Durable list; delete is the dismissal | Snooze (shortcut `H`; reappears as new+unread at time; **new activity before the time cancels the snooze**), mark read/unread, delete read |
| PagerDuty | Triggered = needs a human; escalation repeats until acknowledged | Resolved (manual or auto-resolve timeout) | **Ack timeout re-triggers**: acknowledged incident returns to triggered and escalation resumes (a built-in "gone quiet" watchdog on the operator's own commitment) | Per-incident log of trigger/ack/escalate/resolve events | Durable incident record; notifications are transient fan-out | Acknowledge (pauses escalation, owns it), resolve; ack is time-boxed |
| Cursor background agents | Web/mobile + Slack; forum requests for "needs input/approval" push show it was not covered at the time of the post | Slack message with PR link on completion; PWA push on completion; "@Cursor list my agents" | Not documented | Agent run history viewable offline in the PWA | Run list is durable; Slack/push transient (forum reports missed Slack notifications, i.e. the exact "missed notification" gap here) | Not documented |
| Conductor | Notifications added Jan 2026 for "Claude has questions or awaits plan review" (changelog 0.28.7); no completion notification found | Diff viewer with per-file "viewed" that auto-reverts to "Ready for review" when the file changes | Not found | Chat + diff | Workspace list durable; archive workspace when done | Archive workspace; files mark as viewed |
| Vibe Kanban | Workspace state "Needs Attention: agent finished or needs input" | Same bucket as needs-input | Separate "Idle: waiting" bucket | Kanban card + task attempts | Board is durable | Move card between columns |
| Devin (API) | Session `status_detail` values: working, waiting_for_user, waiting_for_approval, finished; suspended reasons incl. inactivity | `finished` | `suspended` due to inactivity | Message log | API-visible state, durable | Sending a message auto-resumes a suspended session |
| Claude Code hooks (signal source, not a product surface) | `Notification` hook with matchers `permission_prompt`, `idle_prompt`, `elicitation_dialog` | `Stop` hook fires after every completed response; `idle_prompt` = finished and waiting for next prompt, less noisy but can be delayed | n/a | n/a | n/a | n/a |

Sources:
- GitHub: https://docs.github.com/en/subscriptions-and-notifications/concepts/about-notifications and https://docs.github.com/en/enterprise-server@3.0/account-and-profile/managing-subscriptions-and-notifications-on-github/viewing-and-triaging-notifications/managing-notifications-from-your-inbox (VERIFIED via search summary).
- Linear: https://linear.app/docs/inbox, https://linear.app/docs/snooze, https://linear.app/changelog/2021-06-17-inbox-snooze-and-easier-issue-merge (VERIFIED via search summary).
- PagerDuty: https://support.pagerduty.com/docs/incidents, community thread https://community.pagerduty.com/ask-a-product-question-2/acknowledge-button-on-ios-seems-to-only-snooze-733 (ack-timeout re-trigger; default timeout figure not confirmed).
- Cursor: https://docs.cursor.com/background-agent/web-and-mobile, https://docs.cursor.com/slack, https://forum.cursor.com/t/no-slack-notification-background-agent/152122, https://forum.cursor.com/t/push-notifications-mobile-desktop-when-agent-needs-user-input-or-approval/148230.
- Conductor: https://www.conductor.build/changelog/0.28.7-change-target-branch, https://www.conductor.build/docs/index (no completion notification found; absence is UNVERIFIED).
- Vibe Kanban: https://virtuslab.com/blog/ai/vibe-kanban/ and https://www.vibekanban.com (state model quoted from a secondary source; UNVERIFIED against vendor docs).
- Devin: https://docs.devin.ai/zh/api-reference/v3/sessions/get-organizations-session (read via Chinese locale; translated by the search tool). No English Slack-notification doc found.
- Claude Code hooks: https://code.claude.com/docs/en/hooks-guide (matcher names from secondary sources and a zh page; confirm against the English hooks reference before relying on them).
- Not covered (search did not reach them, UNVERIFIED): Opsgenie, Superhuman, Slack Activity, OpenHands, Sculptor, Claude Squad, Claude Code desktop/web, CI dashboards.

### Patterns that transfer (INFERRED from the table)

1. **Two buckets, not one**: GitHub splits participating from watching; Vibe Kanban splits "Needs Attention" from "Idle". The previous project already adopted this logic for idle (see section 3).
2. **Done is a state, not a delete**: GitHub keeps `is:done` queryable; this satisfies "durable until acknowledged, recoverable afterwards".
3. **Snooze that auto-cancels on new activity** (Linear) is the safest snooze semantics for a single operator: nothing stays hidden after the agent produces something new.
4. **Ack with an expiry** (PagerDuty) maps to "intentional pause": ack = I know, I will get to it; if it stays unchanged past a timeout, it returns. Maps onto the existing `LastAcknowledged` snooze (section 2).
5. **Re-open on change** (Conductor's viewed files revert on change) matches the existing rule that ack is dropped when output genuinely changes.
6. **Completion and needs-input must be separate reasons**: Cursor and Conductor both ship them separately and Cursor users report the missed-notification gap that motivates this project.
7. **Prefer agent-emitted signals over screen scraping**: Devin exposes explicit `waiting_for_user`; Claude Code exposes `Stop` / `idle_prompt` / `permission_prompt` hooks. For prose questions the Stop hook plus last transcript message is the reliable "turn ended" signal for Claude; other programs (aider, agy, pi) will need per-program fallbacks.

## 2. Codebase inventory (overlapping surfaces)

| Surface | Path | What it is | Overlap |
|---|---|---|---|
| Review Queue | `web-app/src/app/review-queue/page.tsx` (458 lines), `components/sessions/ReviewQueuePanel.tsx`, `ReviewQueueNavBadge.tsx`, `ReviewQueueBadge.tsx` | Sessions flagged by `DefaultStatusDeterminer` (`session/review_queue_determiner.go`); fed by `session/review_queue_poller.go`; has keyboard shortcuts and help modal | The target surface |
| Notifications tray/page | `web-app/src/app/notifications/` (`NotificationsPage.tsx`), delivery gate `server/deliverygate`, `get_notification_history` MCP tool | Durable notification records (`IsRead`, dedup by `(sessionID, type)`), hidden-session gate | Overlaps "durable place to find a missed notification" |
| Up Next | `web-app/src/app/unfinished/` (`UnfinishedTab.tsx`, `components/unfinished/UpNextTabs.tsx`, `lib/hooks/useUpNextTab.ts`) | Radix tabs: `prs` (default), `stuck`, plus others (worktrees, backlog); per-tab badges; URL `?tab=` > localStorage > `prs` | Candidate home for consolidation; tab shell already supports badge slot |
| History | `web-app/src/app/history/page.tsx` (450 lines) | Claude conversation history search/fork (`ClaudeHistoryEntry`) | NOT a session event log; different data |
| Logs | `web-app/src/app/logs/page.tsx` (212 lines) | Server log viewer with patterns | NOT session-event-oriented |
| NotificationToast / tray | `notification-tray-and-hidden-session-gate` ADR-006/007/009 | Non-modal overlay, in-repo toast stack, mobile docks at top of session page | Transient layer |
| Session cards | `SessionCard.tsx:1272` `Updated {formatTimeAgo(session.updatedAt)}`; `SubStatusChip.tsx`, `SessionRow.tsx:352` | Status chips and relative-time | Per-session surfacing; `touchUpdatedAt` has 12 non-test call sites in `session/*.go` (grep count) |
| Backlog activity log | `session/ent/schema/backlog_activity_note.go` (per ADR) | Per-backlog-item append-only notes | Pattern precedent for an event table, not session-scoped |
| Ack/snooze mechanics | `session/review_state.go`, `review_queue_poller.go:777,893-895` | `LastAcknowledged` "acknowledgment snooze"; re-appears when output genuinely changes | The existing snooze semantics (implicit, ack-based). No user-facing timed snooze found (grep "snooze" hits only these internal comments) |

## 3. Prior decisions (do not re-litigate; directly relevant ones first)

- **notification-revamp ADR-002 (Accepted 2026-09-04): idle items are removed from the Review Queue on the frontend** (`isReviewQueueVisible`: `reason !== "idle"`), `Determine()` unchanged, and idle shows as a `SubStatus.IDLE` chip on the Sessions list. **This conflicts with requirement #2** (finished and waiting for context should be visible). Resolution options for the plan/ADR: (a) introduce a new reason (for example `finished_awaiting_direction`, distinct from `ReasonIdle`) that is visible and keep plain idle hidden; (b) supersede ADR-002 explicitly. Option (a) honors the prior decision's GitHub participating/watching logic. Source: `project_plans/notification-revamp/decisions/ADR-002-idle-items-removed-from-review-queue.md`.
- **notification-revamp ADR-001**: dedup key is `(sessionID, notificationType)` regardless of read state; a recurrence resets `IsRead` except for `auto_approved`; retention keys off `LastOccurredAt`. Any new attention reasons must be new notification types or they collapse into existing records.
- **notification-tray-and-hidden-session-gate**: single delivery gate at bus publish (ADR-001), policy keyed on type with fail-open (ADR-002), five live-settable flags (ADR-004), hidden-session read-only enforced server-side (ADR-005), tray is a non-modal overlay (ADR-006), bulk actions limited to informational rows (ADR-008), audited reply to pending question (ADR-010). An event log must hook after or alongside the gate so suppressed events are either logged with a "suppressed" flag or deliberately not logged.
- **review-queue-event-driven ADR-001**: coalescing capacity-1 channel per controller on `OnOutput` instead of polling; new detection must ride this path (matches the NFR of no new tight loop).
- **review-queue-state-detection ADR-001**: working-state enum rather than bool. New states should extend that enum.
- **review-queue-severity ADR-001**: risk-level vocabulary; priority and risk badges already shipped.
- **backlog-item-activity-log ADR-001/002**: sibling ent table (not extending progress note); single-entry event payload, not whole-item snapshots. Reuse this shape for a session event table: append-only, indexed `(session_id, created_at)`, small payload.
- **up-next-tabs-pr-nudge ADR-003**: controlled Radix tabs with badge slot, deterministic tab resolution, inactive panels unmount, stream hooks lifted into `UnfinishedTab`. Adding an "Attention" tab means following this shell; the default tab stays `prs` unless changed deliberately, and a lifted attention stream is needed for the badge.
- **review-session-notification-cleanup ADR-001**: notification record has a session-scoped field (cleanup on session end); relevant to durability vs cleanup of items for deleted sessions.
- **smart-notification-dedup**: research under `project_plans/smart-notification-dedup/research/` (not read in depth here).

## 4. Edge cases and unstated needs

1. **Snooze / intentional pause**: operator parks a session on purpose ("I will answer tonight"). Today only implicit ack-based snooze exists. Needs an explicit decision: timed snooze (Linear style, auto-cancel on new agent output) versus ack-until-change. Without it, a durable inbox becomes noise and the operator stops trusting it.
2. **Done vs dismiss vs resolved-by-state**: item auto-clears when the underlying state changes (agent resumes working) versus operator-dismissed; keep dismissed items queryable (GitHub `is:done`) so "missed" is recoverable.
3. **Dedup with the notification store**: ADR-001 collapse means a recurring type shows one record with reset read state; the event log is a separate append-only stream and must not be derived from collapsed notification records, or counts and history are lost. Also avoid double rows for the same transition from poller path and event-driven path.
4. **Hidden/system sessions**: complete/idle/stale suppressed by design; gate fails open. Decide whether hidden sessions appear in the event log (recommend log but not surface) so history is complete without floods.
5. **Archived / paused / hibernated sessions**: `noarchivedrevival` forbids reviving as a side effect; surfacing a suspended session must be read-only with an explicit user "resume" action. Log the pause transition even though the poller skips those statuses.
6. **Multi-program**: Claude has hooks (Stop / idle_prompt) for a reliable turn-ended signal; aider, agy, pi need screen heuristics or no prose-question support. Per-program coverage and a "cannot detect" state rather than silent omission.
7. **Prose question false positives**: a final message ending in "?" is not always a question to the operator (rhetorical, summary). Prefer a low-priority "finished, awaiting direction" over a high-priority "question" when unsure; confidence-labelled reason.
8. **Stale clock**: define "gone quiet" on last meaningful output, not `UpdatedAt` (bumped by `touchUpdatedAt` at 12 sites). Show a stable server timestamp so reload and restart do not reset it; persist it (INFERRED root cause for the reload issue needs verification by the Stack/Architecture research).
9. **Mobile**: tray docks top on session page (tray ADR-009); an inbox row needs touch-sized actions (done/snooze) and swipe or menu; Up Next tab strip on narrow widths with a fourth/fifth tab may overflow; must test both form factors.
10. **Restart durability**: event log in ent (additive, not generated-code committed), bounded retention per session and global (NFR); redact terminal content (secrets).
11. **Counts and badges**: nav badge (`ReviewQueueNavBadge`) and per-tab badge must derive from one array (as ADR-002 did) to avoid disagreement; unread vs open distinction.
12. **Migration**: deep links to `/review-queue`, keyboard shortcuts, onboarding modal (`review-queue-onboarding-modal`), e2e specs (`review-queue-e2e-onboarding`), feature registry, `route-meta.ts` row; use a flag-gated redirect.
13. **Bulk actions**: tray ADR-008 limits bulk to informational rows server-side; an inbox "mark all done" must not clear needs-input items.
14. **Away operator / notification channels**: web push and Slack are transient; inbox is the durable fallback. Cursor forum reports show the missed-notification failure is common in this product class.
15. **Self-observability**: log when a needs-operator session produces no item (today `shouldSkipSession` silently skips), and expose a coverage-gap count.

## 5. Implication for the alternatives in requirements.md (INFERRED)

- Industry leans toward one durable inbox with filters plus transient alerts (GitHub, Linear, PagerDuty), i.e. closest to alternative D. A single operator does not need separate pages per reason.
- Vibe Kanban's single "Needs Attention" bucket plus separate "Idle" supports keeping the attention list narrow and making idle a chip, consistent with ADR-002.
- Replacing the Review Queue with the tray alone (B) is weakest: the tray's records are collapsed by dedup (ADR-001) and designed as notifications, not per-session state.
- Open gap: no evidence gathered on how products present "recently finished" or "unchanged for a while" views; none of the surveyed docs expose one (UNVERIFIED beyond Devin's inactivity-suspend state).
