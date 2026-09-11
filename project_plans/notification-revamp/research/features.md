# Research: Feature Landscape — notification-revamp

Phase 2 research agent 2 (Features). Covers: in-repo comparables to copy from,
industry comparables mapped to the 6 in-scope items, edge cases the design
must handle, and unstated needs implied by the explicit requirements.

## 1. Comparable features already in this codebase

**Copy from, don't reinvent — these solve grouping/dedup/audit-trail for a
sibling "triage" surface (the backlog board) and should be the template:**

- **`web-app/src/components/backlog-stuck/StuckItemsSection.tsx`** — the
  closest existing analog to "Needs a decision" grouping. Patterns worth
  copying directly:
  - A **fixed, deliberate display order by actionability, not severity**
    (`GROUP_ORDER: StuckReason[]`, lines ~14-42) — the same principle Scope
    item 6 needs for Priority/RiskLevel tiers. Its own comment warns that a
    reason present in the data but missing from this array is silently
    dropped from render while still counting toward the badge — a
    count-vs-list mismatch bug already hit once (backlog/plan-approval-flicker
    fix, 2026-08) and worth guarding against with the same exhaustiveness
    discipline (`Record<Reason, T>` maps compile-checked, ordering arrays are
    not).
  - **Synthetic aggregate rows excluded from their own sub-counts**
    (`isEscalationReason` for `MULTIPLE_REASONS`/`BOUNCE_CAP_EXHAUSTED`) — an
    analog exists for notification-revamp: an "Auto-resolved by rule" audit
    entry must not itself be counted as a fresh "needs a decision" item in
    whatever aggregate badge/count feeds the top-of-page state.
  - Collapsible-by-default with a toggle handler pattern is already proven
    (`onToggle`/`isOpen` state lives in the parent, section is a dumb
    presentational component) — the same shape as `AutoHandledSection` below.

- **`web-app/src/lib/utils/notificationGrouping.ts`** (`groupNotifications`)
  — **this already implements Scope item 5's core grouping mechanism.**
  Groups by `(sessionId, notificationType)`, takes `count = max(serverCount,
  group.length)` to bridge server-deduplicated single records and legacy
  pre-dedup multi-row data, sorts groups by most-recent representative. This
  is the exact shape needed once `NotificationHistoryStore.Append()` (Scope
  item 1) starts bumping `OccurrenceCount` past first-read instead of forking
  new rows — **the grouping utility likely needs zero changes**; the bug is
  entirely server-side (`store.go`'s `findUnreadDuplicate` only matches
  *unread* records, per the ADR-003 comment at store.go:21-27). Verify this
  utility already tolerates read records in a group before assuming a
  rewrite is needed.

- **`web-app/src/components/ui/NotificationItem.tsx`'s `AutoHandledSection`**
  — a working, shipped "collapsible section of classifier-resolved items"
  component (lines 282-333), already rendering `classifier_rule_name` and a
  ✓/✗ decision glyph per item. Scope item 5 says "extend it to include
  rule-reconciled items" — this is literally an extension point: reconciled
  items just need the same metadata shape (`approval_decision`,
  `classifier_rule_name`) plus a way to distinguish "auto-decided live" from
  "auto-resolved after the fact by a rule change" in the copy (e.g. a
  distinct label, not a distinct component).

- **`session/review_state.go`'s `IsAcknowledgedAfterOutput()`** — the
  existing Stale-pattern acknowledgment suppression Scope item 2 asks to
  extend to idle. It's already **generic and reason-agnostic**: it compares
  `lastAcknowledgedNs > lastMeaningfulOutputNs`, with no Stale-specific state.
  `review_queue_determiner.go`'s idle branch (~L257-266) currently has no
  call to it at all, unlike the Stale branch at L281 (`alreadyAcknowledged :=
  inst.IsAcknowledgedAfterOutput()`). This suggests Scope item 2 may be a
  small, surgical fix — call the same existing method from the idle branch —
  rather than new acknowledgment-tracking infrastructure. Confirm in Phase 3
  planning whether idle's semantics (`UpdatedAt`-based) vs stale's
  (`LastMeaningfulOutput`-based) timestamp sources are compatible with the
  same ack-comparison method, or need their own ack timestamp field.

- **`server/services/claude_settings_watcher.go`'s `onReload` callback**
  (`func(rules []classifier.Rule, origin string, notify bool)`, used by the
  already-shipped `dynamic-rule-reload` project) — **this is the answer to
  Open Question #2.** It's the existing single choke point where rule
  changes are already observed and rules already get re-applied at runtime;
  `Reload()`/`reloadLocked()` (lines 74-149) is called both from a
  file-watcher goroutine and from the `ReloadClaudeSettingsRules` RPC
  handler. Hooking rule-reconciliation here (rather than inventing a second
  trigger point on `upsert_approval_rule`) reuses a path that's already
  proven to run exactly-once-per-change with existing dedup/locking (see its
  own comment on avoiding double-invocation from goroutine + RPC racing).

- **`project_plans/backlog-session-lifecycle-ux/decisions/ADR-001-respawn-event-audit-trail.md`**
  — **direct architectural precedent for the reconciliation audit trail**
  Scope item 4 requires. That ADR's decision for `RespawnEvent` (append-only,
  embedded on the parent entity, capped eager-load, best-effort writes that
  never fail the primary operation) is close to a template for a
  `ReconciliationEvent`-shaped record here: append-only, one row per
  auto-resolve, embedded/joined onto the notification or approval record it
  affected, writes best-effort (an audit-log failure must never block the
  actual auto-resolve). Its "Alternatives Considered" section explicitly
  rejects a separate paginated RPC and a shared table with a `kind`
  discriminator for the *same* reasons that would apply to a
  notification-reconciliation audit record — reuse that reasoning rather than
  re-deriving it.

- **No existing "reconcile pending items against changed rules" logic
  anywhere in the repo.** `grep -rn reconcil` across `.go`/`.ts`/`.tsx` turns
  up only the unrelated backlog stuck-item reconciliation
  (`reconcileMultiReasonEscalation`) — confirms Failure Mode 3 is a genuine
  gap, not something to search harder for.

## 2. Industry comparables mapped to the 6 in-scope items

| In-scope item | Comparable pattern | Source |
|---|---|---|
| 1. Dedup regardless of read state | **PagerDuty `dedup_key`**: subsequent events with a matching key roll into the *same* incident (bump/update) as long as it's unresolved — the analog for "unread" here is "unresolved," not merely "unseen." Reading a PagerDuty incident doesn't fork a new one on the next alert; only *resolving* it does. This maps directly onto Scope item 1's fix: key off "not yet resolved" (unread), not literally "unread == unseen," and once read, treat the *next* occurrence as reopening/bumping the same record rather than forking. | [PagerDuty Alerts docs](https://support.pagerduty.com/main/docs/alerts) |
| 2. Idle ack suppression that "sticks" | **Linear inbox snooze**: hides a notification until a chosen time *or* until new activity occurs on the underlying issue — auto-un-snoozes on new signal rather than staying suppressed forever. Maps onto the idle-ack requirement: suppression should last "until state materially changes" (per the Success Metric), exactly Linear's "snoozed until next real activity" semantics, not a fixed-duration mute. | [Linear Inbox docs](https://linear.app/docs/inbox) |
| 3. Working-state detection (not idle/stale mid-turn) | No single vendor comparable — this is closer to PagerDuty/Alertmanager's problem of avoiding false "resolved→refired" flapping (see Edge Cases below) than to an inbox UX pattern. The relevant transferable idea is **debouncing on a condition, not a single sample** (Alertmanager's `group_wait`/`repeat_interval`). | [Alertmanager grouping](https://github.com/prometheus/alertmanager/issues/1587) (context, not endorsement) |
| 4. Rule-reconciliation auto-resolves stale pending approvals | **GitHub "Done" semantics + re-fire**: "Done" removes from inbox while remaining subscribed, and *will reappear* if the same trigger fires again — i.e., auto-resolution is not a permanent unsubscribe, it's "resolved for now, given current evidence." Maps onto Scope item 4: auto-resolving a pending approval via rule reconciliation should behave like GitHub's Done — the item is closed with a visible cause, not silently forgotten, and a genuinely new occurrence of the same underlying request still surfaces again through the normal approval flow. | [GitHub notifications docs](https://docs.github.com/en/subscriptions-and-notifications/concepts/about-notifications) |
| 5. Notifications IA reboot (needs-decision on top, grouped/collapsed activity) | **GitHub Participating vs Watching** split, and **Linear's category grouping** ("status-changes" bundles completions, cancellations, priority changes, blocking-relationship changes into one group). Maps onto Scope item 5 directly: "needs a decision" ≈ Participating (things requiring you), "recent/completed activity" ≈ Watching (FYI), and the grouping key should be semantic (what kind of event, which session) the way Linear groups by category rather than raw chronology. | [GitHub notifications](https://docs.github.com/en/subscriptions-and-notifications/concepts/about-notifications), [Linear Notifications docs](https://linear.app/docs/notifications) |
| 6. Review Queue IA reboot (Priority/RiskLevel visual tiers, idle moves to session chip) | Same GitHub Participating/Watching split, applied to the review queue rather than notifications: idle "ready for next task" is not an attention item at all (much like GitHub doesn't put your own passive Watching activity above @mentions) — it belongs as passive session-list status, matching Open Question 3's leaning toward removal. | [GitHub notifications](https://docs.github.com/en/subscriptions-and-notifications/concepts/about-notifications) |

## 3. Edge cases and failure modes the design must handle

- **Flapping (idle → working → idle rapidly).** Any working-state detector
  gating idle/stale classification (Scope item 3) needs debounce, not a
  single-sample check — otherwise a session that flickers between
  `StatusIdle` and `StatusProcessing` every poll tick (2s, per
  `review_queue_poller.go`) will flip in and out of the review queue on
  every tick, which is worse than the current "everything shows up" problem
  because it also breaks any "acknowledge until state changes" suppression
  (item 2) — the ack would appear stale on literally the next tick. The
  codebase already has one instance of exactly this problem being fixed:
  `detection/detector.go`'s comment on `WaitingForAgentStuckThreshold` — a
  `WaitingForAgent` status is trusted as "active" only while recently
  updated, falling back to time-based staleness once it's been stuck too
  long, specifically because a background command that never decrements
  would otherwise wedge the state permanently. The same
  "trust-the-signal-but-only-briefly, then fall back to time-based" idiom is
  the template for whatever working-state check Scope item 3 adds — apply
  a minimum-duration threshold before flipping the *review-queue-visible*
  classification even if the underlying detected status flips faster.

- **Rule deleted after items were reconciled by it.** Reconciliation events
  (Scope item 4's audit note, "Auto-resolved by rule: `<name>`") must record
  a **snapshot of the rule's identifying info at reconciliation time** (name,
  ID, decision), not a live reference/foreign key to the rule. If the audit
  note is a live pointer and the rule is later deleted, the note becomes
  either stale (dangling reference silently rendered as blank/"unknown
  rule") or actively wrong (if rule IDs are ever reused). This mirrors
  `RespawnEvent`'s explicit "loose string reference, not a hard FK" choice
  (ADR-001, Alternative Considered #3) for exactly this reason — a historical
  audit record must survive the referenced entity's deletion. Because the
  underlying decision (allow/deny) already happened and the session already
  proceeded, there's no live behavior to "undo" when the rule is deleted —
  only the audit trail's *display* needs deletion-safety, not the actual
  session state.

- **Occurrence counts across a server restart (file-backed persistence).**
  `NotificationHistoryStore` is JSON-file-backed
  (`notifications.json`-equivalent per the Constraints section) and already
  has a documented migration path for exactly this kind of
  format-evolution risk: `deduplicateExisting()` runs once at store
  construction to consolidate pre-dedup-logic duplicate unread records
  (store.go, called from the constructor). Any change to Scope item 1's
  dedup key or the meaning of "unread" needs the equivalent: a one-time
  migration pass over already-persisted records so `OccurrenceCount` doesn't
  silently reset to effectively-1-per-row for data written under the old
  scheme. `OccurrenceCount == 0` from old JSON is already treated as "1" by
  convention (documented in the struct comment) — preserve that convention
  rather than introducing a second "count is missing vs. count is legitimately
  1" ambiguity.

- **Two rules that could both now auto-decide the same pending item — which
  wins?** Verified in `pkg/classifier/classifier.go`: `Classify()` /
  `classifyInternal()` (line 456+) is a single deterministic function that
  evaluates rules in one pass and returns one `ClassificationResult` — there
  is no notion of "multiple rules match, pick one" at the reconciliation
  layer, because rule *evaluation order/precedence* is already resolved
  inside `Classify()` itself (first-match-wins or explicit priority,
  whichever `classifyInternal`'s existing logic does — reconciliation just
  needs to call the same `Classify()` used for live requests, per the
  Feasibility Risk about not duplicating classifier logic). The audit note
  should record whichever single `RuleID`/`RuleName` `Classify()` returned
  for that reconciliation pass — no new conflict-resolution logic is needed,
  only re-running the existing function and trusting its existing
  precedence. Where this *can* still go wrong: if reconciliation is
  triggered separately per-rule-edit rather than once per reload batch, two
  near-simultaneous rule upserts could each independently re-run
  reconciliation and race on which one's audit note "wins" for the same
  item. Hooking into the single `onReload` callback (see §1) rather than a
  per-upsert trigger avoids this by construction — one reconciliation pass
  per reload, using the fully-updated rule set, not N races.

## 4. Unstated needs beyond the explicit requirements

The user's actual complaint was "it doesn't tell me what needs my
attention" — once auto-resolution and grouping exist, several follow-on
expectations become natural and should at least be scoped/flagged even if
not built in this pass:

- **Expectation of a visible reconciliation history, not just a live
  toast/note.** The Observability Requirement already mandates a structured
  log event for every auto-resolve (rule name, item ID, before/after
  decision), and Scope item 5 already says the Auto-handled section should
  extend to rule-reconciled items — so the *UI* side of this is in scope.
  What's implicit but not stated: once a user sees "Auto-resolved by rule:
  X" once, they will expect to be able to answer "show me everything this
  rule has ever auto-resolved" later — i.e., a rule-scoped view, not just a
  time-scoped one. `get_notification_history`
  (`server/mcp/tools_notifications.go`) and
  `ApprovalAnalyticsPanel.tsx`/`useApprovalAnalytics.ts` already exist as
  precedent for exposing decision history sliced by rule
  (`RuleID`/`RuleName` is already a first-class field in
  `classifier.ClassificationResult` and already recorded via
  `analyticsStore.RecordFromResult`). Reconciliation events should be
  recorded through the *same* analytics path used for live auto-decisions
  (tag them e.g. `via: "reconciliation"` vs `via: "live"` in metadata) rather
  than a parallel one, so a future "rule impact" view doesn't need a second
  data source. This is not new Scope, just a data-shape decision now that
  avoids a fork later.

- **Expectation that "auto-resolved" is distinguishable from "auto-decided
  live."** The Auto-handled section today (`AutoHandledSection`) presents
  live classifier decisions as a flat list of ✓/✗ items with a rule name.
  Once reconciled items land in the same section, a user will want to tell
  at a glance "this was decided when the request came in" vs. "this sat
  pending for N hours/days and only got resolved once I added a rule" —
  the latter is materially more interesting (it's evidence the rule closed
  a real gap) and risks being visually indistinguishable if reconciled items
  aren't labeled distinctly. This is implied by the Success Metric's own
  wording ("visible 'Auto-resolved by rule: `<name>`' note ... never
  silent") but the *distinction from live auto-approval* isn't spelled out —
  flag for Phase 3 UX design.

- **Expectation that reconciliation is retroactively visible per-rule during
  rule editing, not just after the fact.** `ApprovalRulesPanel.tsx` is where
  a user edits/adds rules today. Once they know editing a rule can
  retroactively resolve pending items, the natural next ask is "show me,
  before I save, how many currently-pending approvals this edit would
  affect" (a dry-run count) — genuinely useful but out of scope per the
  Rabbit Holes section ("scope strictly to re-run existing `Classify()`
  against pending items, not a classifier redesign"). Flagging so Phase 3
  can explicitly defer it rather than it surfacing as scope creep mid-build.

- **Mobile parity for whatever new interaction affordances get added.** Per
  `feedback_mobile_desktop_ux` (standing project instruction): any new
  collapse/expand, snooze-style ack, or priority-tier visual treatment added
  to `NotificationsPage.tsx` / `ReviewQueuePanel.tsx` needs touch-target and
  responsive-layout parity — the requirements doc's own screenshots that
  triggered this project were mobile screenshots ("152 notifications, 107
  review-queue items"), so mobile is not a secondary concern here, it's the
  originating evidence.

## Sources

- [GitHub: About notifications](https://docs.github.com/en/subscriptions-and-notifications/concepts/about-notifications)
- [GitHub: Managing notifications from your inbox](https://docs.github.com/articles/marking-notifications-as-read)
- [Linear Docs: Inbox](https://linear.app/docs/inbox)
- [Linear Docs: Notifications](https://linear.app/docs/notifications)
- [PagerDuty: Alerts](https://support.pagerduty.com/main/docs/alerts)
- [prometheus/alertmanager#1587 — alert grouping discussion (context on flapping/grouping trade-offs)](https://github.com/prometheus/alertmanager/issues/1587)
