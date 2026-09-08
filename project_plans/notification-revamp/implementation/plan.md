# Implementation Plan: notification-revamp

**Feature**: Fix notification/review-queue signal-quality bugs (dedup, idle-ack,
working-state gap, rule-reconciliation) and reboot both pages' IA around
"needs a decision" vs. collapsed informational activity.
**Date**: 2026-09-04
**Status**: Ready for implementation
**ADRs**:
- ADR-001: Notification Dedup Collapses Regardless of Read State (AUTO_APPROVED Carve-Out, Retention Keyed on LastOccurredAt)
- ADR-002: Idle Items Are Removed From the Review Queue, Not Visually Collapsed
- ADR-003: Reconciliation Audit Trail Is a Metadata Stamp on the Original Notification Record, Not a New Record Type
- ADR-004: Rule-Reconciliation Runs Asynchronously After the Rule Swap, Outside `rebuildMu`

---

## Step 0.5 — Creative Pass: Alternatives Considered

Three high-level shapes for the whole project, before committing to one:

**A. Backend-first minimal-diff fixes + reuse-everywhere frontend (chosen).**
Fix each of the four backend bugs as the smallest correct diff to the file
that already owns the behavior (`store.go`, `review_queue_determiner.go`,
`rules_service.go`), and build the frontend IA reboot entirely out of
components/utilities that already exist and are already tested
(`Collapsible.tsx`, `groupNotifications`, `AutoHandledSection`,
`SubStatusChip`, `ReviewQueuePanel`'s `GroupingStrategy`).
**Strength**: every research document (stack.md, build-vs-buy.md) independently
confirmed zero new dependencies and zero new persistence are needed — this is
the lowest-risk path and the only one consistent with the Constraints section
verbatim. **Weakness**: the four fixes stay four independent, unrelated
diffs rather than sharing one new abstraction, so a *fifth* future
signal-quality bug (e.g. `smart-notification-dedup`'s health-alert dedup)
won't automatically benefit from anything built here.

**B. Unified "attention state" engine.** Introduce one cross-cutting
event-sourced domain concept (`AttentionState`) that notifications,
review-queue items, and reconciliation all read/write through, modeled on
PagerDuty's incident lifecycle (triggered → acknowledged → resolved) as one
shared abstraction across all three subsystems.
**Strength**: would give every future signal-quality feature (including the
explicitly-deferred health-alert dedup) a single foundation instead of N
bespoke fixes. **Weakness**: requires a new persistence layer and schema
migration across three existing subsystems that don't share one today —
directly violates the "no new persistence layer... unless research shows
it's unavoidable" constraint, and blows past the 1–2 week Appetite by an
order of magnitude. Rejected — this is exactly the scope creep the Rabbit
Holes section warns against ("Notifications page IA reboot could expand into
a full activity-feed product").

**C. Frontend-only band-aid.** Fix perceived symptoms (grouping, dedup
display, idle-item visual demotion) entirely in the React layer, leaving
`server/notifications/store.go` and `review_queue_determiner.go` untouched.
**Strength**: fastest to ship, zero backend regression risk against
`TestReviewQueue*`. **Weakness**: already explicitly rejected in
requirements.md's own Alternatives Considered — the dedup and idle-reappear
bugs are server-side state bugs; a client fix re-breaks on refresh or on a
second device (the mobile-web screenshots that triggered this project are
exactly a second client hitting stale server state).

**Chosen: A.** Every task below is sized to the smallest diff that fixes its
root cause in the file that already owns it, reusing existing primitives
identified in build-vs-buy.md and ux.md wherever one exists.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `NotificationRecord` | Persisted row in `NotificationHistoryStore`: one `(sessionID, notificationType)` event, deduplicated with an `OccurrenceCount`. | `server/notifications/store.go:34-51`. |
| `OccurrenceCount` | How many deduplicated occurrences a `NotificationRecord` represents; 0 (old JSON) means 1. | Already exists; this project makes it authoritative past first-read. |
| `LastOccurredAt` | Timestamp of the most recent occurrence of a deduplicated record, distinct from `CreatedAt` (first occurrence). | Becomes the retention-cutoff key (ADR-001). |
| `findDuplicate` | Renamed from `findUnreadDuplicate` — matches `(sessionID, notificationType)` regardless of read state. | Item 1's core fix. |
| `notifTypeAutoApproved` | The `NOTIFICATION_TYPE_AUTO_APPROVED` (13) sentinel type; records of this type are written pre-read and must never flip back to unread on recurrence. | `store.go:29-30`; the ADR-001 carve-out. |
| `ReviewItem` | One entry in the in-memory review queue: a session plus why it needs attention (`Reason`, `Priority`, `Context`). | `session/queue/queue.go`. |
| `AttentionReason` | Enum of why a `ReviewItem` exists (`ReasonIdle`, `ReasonStale`, `ReasonApprovalPending`, etc.). | `session/queue/queue.go`, re-exported via `session/review_queue.go`. |
| `Priority` | Urgency enum for a `ReviewItem`: `PriorityUrgent`(1) > `PriorityHigh`(2) > `PriorityMedium`(3) > `PriorityLow`(4). | `session/queue/queue.go:51-58`. |
| `IsAcknowledgedAfterOutput` | Pure, lock-free predicate: has the user acknowledged this session more recently than its last meaningful output. | `session/review_state.go:172-190`; already used for `ReasonStale`. |
| `suppressedByAck` | New shared helper on `DefaultStatusDeterminer` wrapping `IsAcknowledgedAfterOutput`, used by both Idle and Stale so the suppression rule lives in one place. | New in this plan (Task 1.2.1a). |
| `detection.DetectedStatus` / `IdleState` | Existing shared vocabulary the review-queue determiner already switches on (`StatusIdle`, `StatusWaitingForAgent`, `IdleStateTimeout`, etc.). | `session/detection/`; confirmed already fully wired, not a new state machine (architecture.md §3). |
| `waitingForAgentStuckThreshold` | 30-minute grace period during which `StatusWaitingForAgent` is trusted as real background activity before falling back to time-based idle checks. | `session/review_queue_determiner.go:18`; already exists for the no-controller path, extended to the controller-active path in this plan. |
| `PendingApproval` | An escalated, awaiting-human-decision tool-call request, persisted in `ApprovalStore`. | `server/services/approval_store.go:21-49`. |
| `ClassificationResult` | Pure output of `RuleBasedClassifier.Classify()`: `Decision` (`AutoAllow`/`AutoDeny`/`Escalate`), `RuleID`, `RuleName`, `Reason`. | `pkg/classifier/classifier.go:46-52`. |
| `RulesService.rebuildMu` | Mutex serializing the two existing rule-rebuild paths so a lost-update race can't drop one side's rule change. | `server/services/rules_service.go:39-42` (ADR-002 of `dynamic-rule-reload`). |
| `reconcilePendingApprovals` | New `RulesService` method: after any rule rebuild, re-run `Classify()` against every currently-pending, `Escalate`-sourced `PendingApproval` and auto-resolve any that now decide. | New in this plan (Epic 2.2). |
| `ResolveApprovalReconciled` | New `ApprovalService` method: resolves a pending approval on reconciliation's behalf, reusing `ResolveApproval`'s exact guard/stamp/broadcast logic, then stamps `classifier_rule_name`/`reconciled` metadata. | New in this plan (Task 2.1.1c). |
| `reconciled` metadata key | `"true"` on a `NotificationRecord` when its approval was resolved by rule-reconciliation rather than a live decision. Read via the shared `NotificationRecord.IsReconciled()` (Go) / `isReconciledNotification()` (TS) accessors, never an inline string comparison, so the four read sites can't drift from the one write site. | New in this plan; the audit-trail signal (ADR-003). |
| `ApprovalStore.humanResolving` / `MarkHumanResolving`/`ClearHumanResolving`/`IsHumanResolving` | Mutex-guarded set of approval IDs with a human `ResolveApproval` call currently in flight. Reconciliation checks it before ever attempting to resolve an item and backs off (`connect.CodeAborted`) rather than racing `Resolve()`'s mutex — the enforcement mechanism for research/ux.md's "favor the human, not the automation" mandate. | New in this plan (Task 2.1.1c). |
| `connect.CodeAborted` | Reserved, in this service, exclusively for "reconciliation lost the race to an in-flight human decision" — distinct from `connect.CodeFailedPrecondition`, which is reserved for the CI-red guard and "lost the race to a *different* concurrent reconciliation pass" (disambiguated at the call site via `IsApprovalPending`, since both share that code). | New in this plan (Task 2.1.1c / Task 2.2.1a). |
| `RemovalInfo` | New value object carrying why a `ReviewItem` was removed (`Reason()`, optional `RuleName()`), replacing the always-`"user_action"` string the observer interface hands back today. Unexported fields; constructed only via `UserActionRemoval()`/`AutoResolvedByRuleRemoval(ruleName)` so the Reason/RuleName pairing invariant is compiler-enforced, not just documented. | New in this plan (Task 2.3.1b). |
| `isReviewQueueVisible` | New frontend predicate: false for `Reason === "idle"`. Single source of truth so the panel's list and every nav-badge count agree (ADR-002). | New in this plan (Task 3.2.2a). |
| `isActionableNotification` | New frontend predicate distinguishing "needs a decision" (approval_needed, question, error, task_failed, warning) from informational types, replacing the exclusion-list bug in `notificationTypeFilter`'s "info" case. | New in this plan (Task 3.1.1a). |
| `NeedsDecisionSection` | New always-on-top component on the Notifications page listing unread actionable notifications; the project's success condition when empty. | New in this plan (Task 3.1.2a). |
| `SubStatusChip` (`SubStatus.IDLE`) | Existing chip component already rendering "● Idle"; currently filtered out of `SessionRow` by name. | `web-app/src/components/sessions/SubStatusChip.tsx:137-147`; `SessionRow.tsx:352`. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Notification dedup key | Widen existing predicate in place (Specification-style boolean predicate, not a new abstraction) | PoEAA (Domain Model — keep business rule where the data lives) | A generic dedup/TTL cache library | build-vs-buy.md: no concurrency-scale problem exists here; a library replaces nothing, only adds a dependency for a ~4-line diff. |
| Idle/Stale ack-suppression | Extract-and-share one predicate (`suppressedByAck`) called from both reason sites, replacing two near-duplicate inline checks | Refactoring (Extract Method) | Bespoke, reason-scoped suppression state per reason (a `map[AttentionReason]time.Time`) | Task explicitly forbids "special-casing" per reason; `IsAcknowledgedAfterOutput` is already reason-agnostic — a new per-reason store would duplicate state that already exists once. |
| Rule-reconciliation entry point | Reuse existing `Classify()` pure function + existing `ApprovalService.ResolveApproval` Facade | GoF Facade (single entry point for "resolve an approval," live or reconciled) | New reconciliation-specific classification/resolution logic | build-vs-buy.md + pitfalls.md: bespoke logic would duplicate ~2,850 lines of already-tested classifier logic and, worse, would bypass `ResolveApproval`'s CI-red guard — letting reconciliation auto-approve something a manual click would have been blocked from approving. |
| Rule-reconciliation trigger | Hook into the two existing `rebuildClassifier()`/`rebuildClaudeSettingsRules()` choke points, run async after `rebuildMu` releases (ADR-004) | Observer / Template Method (one shared post-rebuild hook) | A new dedicated `upsert_approval_rule`-only trigger | architecture.md: a new trigger would miss the fsnotify/`ReloadClaudeSettingsRules` paths, which are just as capable of newly covering a pending item; requirements.md's Open Question 2 explicitly asks to prefer the existing reload path. |
| Reconciliation ↔ approval subsystem coupling | Facade: `RulesService` depends on a narrow `reconciliationResolver` interface satisfied implicitly by `*ApprovalService` (which itself owns `ApprovalStore` + `NotificationHistoryStore` access), not the concrete type | GoF Facade | `RulesService` wired directly to `ApprovalStore` + `NotificationHistoryStore` separately; or `RulesService.approvalSvc` typed as concrete `*ApprovalService` | Two direct dependencies duplicate guard logic (CI-red block, metadata stamping, event broadcast) that `ApprovalService.ResolveApproval` already owns; a second caller reimplementing those guards is exactly the "bypasses guards a manual click would be subject to" risk pitfalls.md flags. A concrete-typed `approvalSvc` field would force every reconciliation-loop unit test through a real `ApprovalStore` (architecture-review.md Concern 1) — the interface costs nothing at the one call site and unlocks a lightweight test fake. |
| Human-vs-reconciliation resolution race | A mutex-guarded "claim" set (`ApprovalStore.humanResolving`) that a human's RPC marks before any other work, checked by reconciliation before it ever attempts to resolve; a losing reconciliation attempt gets a distinct `connect.CodeAborted` and backs off entirely | Guarded Suspension / optimistic-claim (no new lock — reuses `ApprovalStore.mu`) | (a) Leave it unarbitrated, accepted as a documented risk; (b) a time-based debounce window (delay every reconciliation resolve by N ms to let in-flight human RPCs land first); (c) a generation/version counter on each approval | adversarial-review.md's Blocker (iteration 2) rules out (a): ux.md's "favor the human" mandate is explicit, not optional, so an unimplemented mandate must actually be implemented, not just re-documented as accepted. (b) trades correctness for a guessed timeout and still isn't deterministic — the exact "no clear winner" flakiness this fix needs to eliminate for the human's already-arrived case. (c) solves the same problem with more moving parts (version compare-and-swap) than a claim/back-off flag needs for the one caller (reconciliation) that must ever defer. |
| Mid-review-race notification-side labeling | Reuse existing `metadata`-driven rendering branches (`resolvedApprovals` badge, `blockedApprovals` inline message) | Strategy (render branch selected by existing state, not a new state machine) | New WebSocket/event-driven toast component | The existing `useApprovalResolution` hook's `catch` branch and `resolvedApprovals` seeding already re-render correctly once metadata refreshes; a new component would duplicate state that already flows through `notificationHistory`. |
| Mid-review-race review-queue-side labeling | One new optional field on `ReviewQueueItemRemovedEvent` (`auto_resolved_by_rule`), carried by a new `RemovalInfo` value object (unexported fields, smart constructors) through the existing observer interface | Value Object (type-driven design: replace a bare string with a typed carrier whose invariant is compiler-enforced, not comment-enforced) | Encode the rule name into the existing free-text `reason` string via a prefix convention; or a plain exported-field `RemovalInfo{Reason, RuleName}` struct | A prefix-encoded string is primitive obsession — the consumer has to parse it back apart. A plain exported-field struct still lets a caller construct an illegal `{Reason: "user_action", RuleName: "x"}` pairing (architecture-review.md Concern 2) — `UserActionRemoval()`/`AutoResolvedByRuleRemoval(name)` smart constructors close that gap at the two call sites that exist today, matching the explicit "one new field... extend, don't replace" constraint. |
| Idle items in the Review Queue | Backend `Determine()` is unchanged; the frontend filters `Reason === "idle"` out of the panel's rendered list and every count that reads the same array | Specification pattern (one predicate, one place) | Remove `ReasonIdle` from `Determine()` entirely | architecture.md §5: this is "purely a frontend layout decision now, not a data-availability question" — changing `Determine()` risks regressing `TestReviewQueue*`/`StartupScanner.Scan`, other consumers of the same pure function, for no added benefit since the frontend fix alone satisfies Scope item 6 (ADR-002). |
| Idle status surfaced on Sessions list | Reuse existing `SubStatusChip`'s `SubStatus.IDLE` case, remove `SessionRow`'s one-line suppression of it | Reuse existing component (build-vs-buy: reuse over rebuild) | Build a new "session idle" badge component | The chip, its CSS, and its `aria-label` already exist and are already correct (`SubStatusChip.tsx:137-147`) — it is filtered out by exactly one boolean term today. |
| Notification IA sectioning | Two-tier partition (`isActionableNotification`) rendered via existing `CollapsibleGroup`/`CollapsibleSection` | GitHub Participating/Watching split (ux.md) | A new activity-feed-style filtering/search product | Explicitly capped by Rabbit Holes: "anything fancier is a follow-up." |
| `notificationTypeFilter`'s "info" bucket | Allow-list of genuinely-informational types | This repo's own `dynamic-rule-reload` ADR-002 precedent (exclusion lists silently admit future unlisted types) | Patch in the one missing exclusion (`"question"`) | pitfalls.md: patching the single known gap leaves the same failure mode for the *next* new notification type (e.g. this project's own `reconciled`-flagged records); the allow-list is exactly as small a diff and closes the whole class. |

---

## Migration Plan

No schema or data migration. All persistence stays JSON-file-backed
(`notifications.json`, `pending_approvals.json`), unchanged in shape —
every new field this plan adds (`OccurrenceCount`/`LastOccurredAt` behavior
change, two new `metadata` keys, one new optional proto field) is additive
and tolerates old on-disk data via existing zero-value conventions (`store.go`'s
documented "`OccurrenceCount == 0` means 1" rule; a missing `metadata` key
reads as `""`/absent, already the norm for `classifier_rule_name` etc.).
`deduplicateExisting()` (Task 1.1.3a) is a one-time in-process migration on
next load, not a schema migration — it already exists and runs on every
`NewNotificationHistoryStore()` call.

## Observability Plan

- **Logs**: one `log.Info` structured event per reconciliation auto-resolve
  (`rule_id`, `rule_name`, `approval_id`, `session_id`, `before_decision:
  "escalate"`, `after_decision`) — the Observability Requirement's mandated,
  non-negotiable line (Task 2.2.2a). One `log.Info`/`log.Warn` summary line
  per reconciliation *pass* (`resolved_count`, `skipped_count`,
  `declined_by_ci_guard_count`, `deferred_to_human_count`,
  `lost_to_concurrent_pass_count`, `capped: bool`, `panicked: bool`) so a
  rule edit that resolves many items at once is visible as one line, not
  only as N easy-to-miss individual notifications (pitfalls.md's
  blast-radius concern; Task 2.2.1c) — and so it fires with real partial
  counts even when a mid-pass panic aborts the loop (Task 2.2.1a).
- **Metrics**: none new — this is a single-operator internal tool with no
  existing metrics pipeline for this subsystem; the structured logs above are
  sufficient per requirements.md's Non-functional Requirements.
- **Alerts**: none — no alerting pipeline exists or is being added (Risk
  Control: no feature flag, no public traffic).

## Risk Control

- **Feature flag**: none, per requirements.md's Risk Control section — this
  is an internal tool behind no public traffic.
- **Rollback procedure**: straightforward `git revert` per change; no
  migration to unwind (JSON-file-backed state, additive fields only). The
  soft cap in Task 2.2.1c (`maxReconcileAutoResolvesPerPass`) is itself a
  cheap in-band circuit breaker if a bad rule reconciles too aggressively —
  no external rollback needed to stop a runaway pass mid-flight, only to
  stop the *next* one (fix or remove the rule).
- **Staged rollout**: none — single operator, land behind a reviewed PR to
  `main` per normal repo flow (this repo's PRs default to ready-for-review,
  not draft, per its own CLAUDE.md).

## Unresolved Questions

- [ ] None blocking. Requirements.md's Open Question 3 ("remove idle vs.
  keep as collapsed tier") is resolved by ADR-002 below (removal, backed by
  requirements' own Scope item 6 text and ux.md's GitHub analogy). Open
  Question 2 (reconciliation trigger point) is resolved by architecture.md's
  finding, restated in ADR-004. Open Question 1 (detection package reuse)
  was already answered as "yes, already wired" by architecture.md §3 before
  this plan started.
- [x] **`smart-notification-dedup` piggyback (requirements.md Out of
  Scope: "fold in only if Phase 3 planning finds it's cheap to piggyback...
  otherwise a separate follow-up") — resolved as: separate follow-up, not
  folded in.** Its scope (health/system alert re-firing — fork-pressure,
  tmux recovery — plus native OS/browser notification auto-dismiss) is a
  different subsystem than this plan touches: this plan's dedup fix
  (Epic 1.1) is entirely inside `server/notifications/store.go`'s
  `(sessionID, notificationType)` collapse logic, while
  `smart-notification-dedup` would need to touch the separate health-alert
  service and native-notification layer, neither of which this plan's Epic
  1.1 tasks read or modify. There is no shared code path a "cheap
  piggyback" could hook into without adding scope (a new dependency edge
  into the health-alert service) beyond what Epic 1.1's Files lists already
  cover — so the piggyback isn't cheap here, and it stays a separate
  follow-up project rather than being folded in.

## Dependency Visualization

```
Phase 1 — Backend Correctness (independent of each other; can implement in any order)
  Epic 1.1 Notification Dedup ─────────────────┐
  Epic 1.2 Idle Ack-Suppression ────────────────┼─── no cross-dependencies
  Epic 1.3 Working-State Gap (WaitingForAgent) ─┘

Phase 2 — Rule-Reconciliation (depends on nothing in Phase 1; self-contained)
  Epic 2.1 Facade wiring (ApprovalService gains
            ListPendingApprovalsInternal,
            ResolveApprovalReconciled, GetByID)
        │
        ▼
  Epic 2.2 reconcilePendingApprovals() core loop
        │         + async trigger (ADR-004)
        │         + audit log + soft cap
        ▼
  Epic 2.3 Mid-review-race UX
        (needs Epic 2.1's metadata stamping
         AND Epic 2.2's reconciliation calling it)

Phase 3 — Frontend IA Reboot
  Epic 3.1 Notifications page IA
        ├─ 3.1.1 "info" filter fix (independent)
        ├─ 3.1.2 NeedsDecisionSection (independent of dedup fix landing,
        │        but Epic 1.1 should land first so occurrenceCount is
        │        authoritative when this ships — soft ordering, not a hard block)
        ├─ 3.1.3 Auto-handled extension (needs Epic 2.1/2.2's `reconciled`
        │        metadata key to exist — hard dependency)
        └─ 3.1.5 NotificationPanel + Clear-all scoping (needs 3.1.2b's
                 optional removeFromHistory prop and 3.1.2e's shared
                 computeScopedMarkReadIds helper to exist first — soft
                 ordering within the epic, not cross-epic)
  Epic 3.2 Review Queue page IA
        ├─ 3.2.1 Priority-tier sectioning (independent)
        └─ 3.2.2 Idle removed from queue + Sessions-list chip (independent
                 of 3.2.1; touches different files)
```

---

## Phase 1: Backend Correctness Fixes

### Epic 1.1: Notification Dedup Regardless of Read State

**Goal**: A repeated `(sessionID, notificationType)` notification always
collapses into one row with a bumped `OccurrenceCount`, regardless of
whether the existing row was already read — except `AUTO_APPROVED` records,
which must stay pre-read. Retention no longer silently deletes a
still-recurring record just because its *first* occurrence aged out.

#### Story 1.1.1: Collapse duplicates regardless of read state, with an AUTO_APPROVED carve-out

**As a** user, **I want** a recurring low-value event (e.g. "Claude turn
complete") for the same session to stay one row with a growing count,
**so that** my notification list stops growing unboundedly past the first
time I read that type of event.

**Acceptance Criteria**:
- A second occurrence of the same `(sessionID, notificationType)` collapses
  into the existing record and flips it back to unread, even if the existing
  record was already read.
  - *Given* a read `NotificationRecord{ID: "n1", SessionID: "sess-a1b2c3",
    NotificationType: TASK_COMPLETE, IsRead: true, OccurrenceCount: 1}`,
    *When* `Append()` is called with a new `TASK_COMPLETE` record for
    `sess-a1b2c3`, *Then* the store still contains exactly one record with
    `ID: "n1"`, `OccurrenceCount: 2`, `IsRead: false`, `ReadAt: nil`.
- An `AUTO_APPROVED` record never flips back to unread on recurrence.
  - *Given* a pre-read `NotificationRecord{ID: "n2", NotificationType:
    AUTO_APPROVED, IsRead: true}` (written via `AppendAutoApproved`), *When*
    the same rule auto-approves the same session+tool again and
    `AppendAutoApproved` is called a second time, *Then* the collapsed
    record has `OccurrenceCount: 2` and `IsRead` remains `true`.
- An `APPROVAL_NEEDED` record's ID-reassignment side effect is unaffected.
  - *Given* an existing `APPROVAL_NEEDED` record `ID: "n3"`, *When* a second
    approval request for the same session arrives with incoming UUID `"n4"`,
    *Then* the collapsed record's `ID` becomes `"n4"` (unchanged behavior)
    and `IsRead` is `false`.

**Files**: `server/notifications/store.go`

##### Task 1.1.1a: Widen the dedup predicate, reset read state on collapse, and run retention on the collapse path too (~5 min)
- Rename `findUnreadDuplicate` → `findDuplicate`; drop the `!r.IsRead` clause
  from its loop condition (`store.go:212-219`).
- Update `Append()`'s call site (`store.go:150`) to call `findDuplicate`.
- In the collapse branch (`store.go:150-165`), after the existing
  `OccurrenceCount++`/`LastOccurredAt`/`Message`/`Metadata`/`Title` updates,
  add: `if record.NotificationType != notifTypeAutoApproved { existing.IsRead
  = false; existing.ReadAt = nil }`.
- **Also call `s.enforceRetention()` on the collapse path (adversarial-review.md
  Minor 1)**: today `enforceRetention()` only runs from the new-record
  branch (`store.go:173`) — the collapse branch returns early at line 164
  without ever sweeping. Since this is the canonical "Claude turn complete"
  recurrence case this project targets, a frequently-recurring record could
  otherwise go a long time between opportunistic sweeps (which only happen
  when some *other*, unrelated new notification arrives anywhere in the
  store). Add `s.enforceRetention()` immediately before
  `s.moveToFront(existing)` in the collapse branch so both paths sweep on
  every `Append()` call, matching the new-record path's behavior — a
  one-line addition, not a design change, since `enforceRetention()` is
  already idempotent and cheap.
- Update the function's doc comment (`store.go:127-137`) to drop the "per
  ADR-003, if the existing record is already read, a new unread record is
  created instead" line — this is now false.
- Files: `server/notifications/store.go`

##### Task 1.1.1b: Update/add tests for the new collapse behavior (~6 min)
- Rewrite `TestAppendDedup_ReadThenNew` (`store_test.go:124-171`) — currently
  asserts a read record forks a new row; assert instead that it collapses
  in place with `OccurrenceCount` incremented and `IsRead` reset to `false`.
  Rename to `TestAppendDedup_ReadThenRecur_CollapsesAndUnreads`.
- Add `TestAppendDedup_AutoApprovedStaysReadAcrossRecurrence`: two
  `AppendAutoApproved` calls for the same session/tool, assert one record,
  `OccurrenceCount: 2`, `IsRead: true`.
- Add `TestAppendDedup_CollapsePathTriggersRetention`: seed a store with an
  old, non-recurring record that's past `MaxNotificationAge`, then trigger a
  collapse via `Append()` on an unrelated `(sessionID, notificationType)`
  pair, and assert the stale record is pruned — this exercises
  `enforceRetention()`'s *real* trigger path via the collapse branch (Task
  1.1.1a), not just the direct-call tests in Task 1.1.2b.
- Files: `server/notifications/store_test.go`

#### Story 1.1.2: Retention keys off `LastOccurredAt`, not `CreatedAt`

**As a** user, **I want** a frequently-recurring notification to never be
silently deleted just because it was first created more than 7 days ago,
**so that** my occurrence count and unread state for an ongoing pattern
don't reset to 1 out of nowhere.

**Acceptance Criteria**:
- A record with an old `CreatedAt` but a recent `LastOccurredAt` survives
  `enforceRetention()`.
  - *Given* a record `CreatedAt: now-10days`, `LastOccurredAt: now-2minutes`,
    `OccurrenceCount: 47`, *When* `enforceRetention()` runs, *Then* the
    record is kept.
- A record with no recent occurrences is still pruned after 7 days.
  - *Given* a record `CreatedAt: now-10days`, `LastOccurredAt: now-9days`,
    *When* `enforceRetention()` runs, *Then* the record is removed.
- A record with `LastOccurredAt: nil` (old, pre-migration data) falls back
  to `CreatedAt` for the cutoff check, matching today's behavior exactly.

**Files**: `server/notifications/store.go`

##### Task 1.1.2a: Change the retention cutoff to `LastOccurredAt` with `CreatedAt` fallback (~3 min)
- In `enforceRetention()` (`store.go:511-522`), replace `r.CreatedAt.After(cutoff)
  || r.CreatedAt.Equal(cutoff)` with a small local: `effective :=
  r.CreatedAt; if r.LastOccurredAt != nil { effective = *r.LastOccurredAt }`
  then compare `effective` against `cutoff`.
- Files: `server/notifications/store.go`

##### Task 1.1.2b: Add a retention regression test (~4 min)
- Add `TestEnforceRetention_KeyedByLastOccurredAt_SurvivesOldCreatedAt` and
  `TestEnforceRetention_NoRecentOccurrence_StillPruned` next to the existing
  `TestEnforceRetention_GatesOrphanSweep_ByOrphanPruneInterval`
  (`store_test.go:828`).
- Files: `server/notifications/store_test.go`

#### Story 1.1.3: `deduplicateExisting()` parity for pre-fix persisted duplicates

**As a** user upgrading to this fix, **I want** notification rows persisted
under the old read-state-forking behavior to consolidate on next load,
**so that** stale forked rows don't linger until they individually age out.

**Acceptance Criteria**:
- On load, a read record and an unread record sharing the same
  `(sessionID, notificationType)` consolidate into one.
  - *Given* on-disk records `{ID:"n5", IsRead:true, OccurrenceCount:1}` and
    `{ID:"n6", IsRead:false, OccurrenceCount:1}`, both `SessionID:
    "sess-a1b2c3"`, `NotificationType: TASK_COMPLETE`, *When*
    `NewNotificationHistoryStore()` loads and runs `deduplicateExisting()`,
    *Then* exactly one record remains, keyed on whichever is newest by
    `CreatedAt`, with `OccurrenceCount: 2`.

**Files**: `server/notifications/store.go`

##### Task 1.1.3a: Group by `(sessionID, notificationType)` regardless of read state in `deduplicateExisting()` (~5 min)
- In `deduplicateExisting()` (`store.go:446-459`), remove the `if r.IsRead {
  continue }` skip so read records are grouped too, not just unread ones.
  Keep the "newest record wins as keeper" logic (`store.go:469-484`)
  unchanged — it already doesn't care about read state.
- Update the function's doc comment ("consolidates unread duplicates") to
  say "consolidates duplicate records regardless of read state."
- Files: `server/notifications/store.go`

##### Task 1.1.3b: Add a migration test (~4 min)
- Add `TestDeduplicateExisting_ReadAndUnreadDuplicates_Consolidate` next to
  `TestDeduplicateExisting_MixedReadUnread` (`store_test.go:514`).
- Files: `server/notifications/store_test.go`

---

### Epic 1.2: Idle Ack-Suppression

**Goal**: Skipping/acknowledging an idle-class review-queue item keeps it
suppressed until the session produces genuinely new meaningful output,
exactly matching the existing Stale-reason behavior — without special-casing
per reason.

**Why this backend fix still matters once Epic 3.2.2 hides all idle items in
the UI regardless of ack state**: it would be reasonable to assume this
epic becomes dead work for the (only) web client once Story 3.2.2
unconditionally filters every `Reason === "idle"` item out of
`ReviewQueuePanel`'s list and counts — but `Determine()` re-adding an
unsuppressed idle item on every poll tick has effects well beyond the
panel's own rendering. `ReactiveQueueManager.OnItemAdded`
(`server/review_queue_manager.go:386-487`) only skips its notification/Slack/
webhook side effects on the `exists==false → true` transition being
suppressed for *Hidden* sessions and `ReasonApprovalPending`; a normal,
visible session's repeatedly-re-added idle item still (a) publishes an
`eventBus` notification event that lands in `NotificationHistoryStore`
(feeding Epic 1.1's own dedup logic — and, worse, *flipping that record back
to unread* on every recurrence, since Epic 1.1 doesn't special-case the
`Reason: "idle"`-sourced `INFO` notification type), (b) fires a Slack ping
when `cfg.Slack.NotifyOnQueueItem` is set, and (c) dispatches a
`queue_item_created` webhook callback. None of those three are touched by
Epic 3.2.2's frontend-only filtering. So this epic is not redundant — it's
the fix for exactly the kind of notification churn this project exists to
eliminate, just one layer below the Review Queue panel itself.

#### Story 1.2.1: Extract a shared `suppressedByAck` helper and apply it to both Idle sites

**As a** user, **I want** skipping an idle session to actually keep it out
of the queue, **so that** I stop seeing the same idle nudge reappear on the
very next poll tick.

**Acceptance Criteria**:
- An idle item acknowledged after its last output does not reappear.
  - *Given* session `sess-a1b2c3` with `LastMeaningfulOutput: t0`,
    `LastAcknowledged: t0+1s` (set via the "skip" action calling
    `MarkAcknowledged()`), *When* `Determine()` runs again at `t0+10s` with
    no new output, *Then* the result is `DetectionActionSkip` (not
    `DetectionActionAdd` with `ReasonIdle`).
- New output un-suppresses it.
  - *Given* the same acknowledged session, *When* new terminal content
    advances `LastMeaningfulOutput` to `t0+15s` (after the `t0+1s`
    acknowledgment) and the session then idles again past the threshold,
    *Then* `Determine()` returns `DetectionActionAdd` with `ReasonIdle`
    again.
- The controller-active and no-controller idle sites both use the identical
  helper — no duplicated inline `IsAcknowledgedAfterOutput()` calls.
- A session whose true reason is `ReasonApprovalPending` or
  `ReasonInputRequired` is never suppressed via the idle path, even if it
  also happens to look idle by terminal-content heuristics.
  - *Given* a session with `statusInfo.ClaudeStatus ==
    detection.StatusNeedsApproval` and an old acknowledgment timestamp,
    *When* `Determine()` runs, *Then* the result is `ReasonApprovalPending`
    at `PriorityHigh` — the idle-suppression check is never reached for this
    session (status-based conditions are checked first, per the existing
    `Determine()` ordering).

**Files**: `session/review_queue_determiner.go`, `session/review_queue_determiner_test.go`

##### Task 1.2.1a: Add `suppressedByAck` and guard both Idle sites (~5 min)
- Add `func (d *DefaultStatusDeterminer) suppressedByAck(inst *Instance) bool
  { return inst.IsAcknowledgedAfterOutput() }` near the top of
  `review_queue_determiner.go`, with the doc comment already drafted in
  architecture.md §2.
- Guard the controller-active `IdleStateTimeout` case (`:178-184`): wrap the
  four-line `reason = ReasonIdle...` block in `if
  !d.suppressedByAck(inst) { ... }`.
- Guard the no-controller time-based fallback (`:259-265`) the same way.
- Files: `session/review_queue_determiner.go`

##### Task 1.2.1b: Refactor the existing Stale block to use the same helper (~3 min)
- Replace the inline `alreadyAcknowledged := inst.IsAcknowledgedAfterOutput()`
  (`:281`) with `alreadyAcknowledged := d.suppressedByAck(inst)`. No
  behavior change — this removes the one duplicate call site so the
  suppression rule genuinely lives in one place.
- Files: `session/review_queue_determiner.go`

##### Task 1.2.1c: Add idle-ack-suppression tests (~5 min)
- Add `TestDefaultStatusDeterminer_IdleAckSuppression_StaysOutUntilNewOutput`
  (no-controller path) and a controller-active variant, mirroring the shape
  of the existing `TestDefaultStatusDeterminer_NoControllerWaitingForAgent_*`
  pair (`:678`, `:706`).
- Add `TestDefaultStatusDeterminer_ApprovalPending_NeverReachesIdleSuppression`
  per the Acceptance Criterion above — asserts the ack-suppressed idle path
  is unreachable when a higher-priority status condition is present.
- Files: `session/review_queue_determiner_test.go`

---

### Epic 1.3: Working-State Detection Gap

**Goal**: The controller-active branch of `Determine()` handles
`StatusWaitingForAgent` the same way the no-controller branch already does,
so a session with a live background task doesn't misclassify as idle just
because the auto-mode footer counter isn't hash-different between polls.

#### Story 1.3.1: Add `StatusWaitingForAgent` handling to the controller-active switch

**As a** user, **I want** a session running a live background task to never
show up as "idle - ready for next task," **so that** I don't get a false
nudge while Claude is still working.

**Acceptance Criteria**:
- A recently-updated `StatusWaitingForAgent` controller-active session is
  removed from consideration, not flagged idle.
  - *Given* `statusInfo.IsControllerActive: true`,
    `statusInfo.ClaudeStatus: detection.StatusWaitingForAgent`,
    `inst.UpdatedAt: now-2minutes` (well within the 30-minute grace period),
    *When* `Determine()` runs, *Then* the result is
    `DetectionActionRemove`.
- A stale `StatusWaitingForAgent` (background task plausibly stalled) falls
  through to the normal idle/stale checks instead of being trusted forever.
  - *Given* the same status but `inst.UpdatedAt: now-45minutes` (past
    `waitingForAgentStuckThreshold`), *When* `Determine()` runs, *Then*
    control falls through to the idle-state switch and staleness check
    exactly as it does today for any other unhandled status.

**Files**: `session/review_queue_determiner.go`

##### Task 1.3.1a: Add the `StatusWaitingForAgent` case to the controller-active switch (~4 min)
- In the `switch` at `:136-164`, add a case mirroring the no-controller
  branch's `:241-252` grace-period logic:
  ```go
  case statusInfo.ClaudeStatus == detection.StatusWaitingForAgent:
      if time.Since(inst.UpdatedAt) < waitingForAgentStuckThreshold {
          return DetectionResult{Action: DetectionActionRemove, ClaudeStatus: claudeStatus}
      }
      // Stale background task — fall through to idle-state handling below.
  ```
- **Accepted limitation, checked rather than assumed (pre-mortem.md #4)**:
  `waitingForAgentStuckThreshold` is keyed off `inst.UpdatedAt`, a
  general-purpose "this instance record changed" timestamp also bumped by
  `Rename()` (`session/instance_terminal.go:99`), saving a note
  (`session/instance_actor_setters.go:397`), workspace/tag changes
  (`session/instance_workspace.go:368`), and other `touchUpdatedAt()` sites
  in `session/instance_state.go` — none of which reflect actual agent
  progress. The obvious narrower alternative,
  `inst.GetTimeSinceLastMeaningfulOutput()` (already used by
  `StaleSessionNotifier`), is *not* actually a fix here: `LastMeaningfulOutput`
  by definition doesn't advance while a session is legitimately
  `StatusWaitingForAgent` with no new terminal content, so keying the grace
  period off it would make the grace period expire immediately for the exact
  case it exists to protect. A correct fix needs a new, narrower field (e.g.
  a dedicated `LastAgentActivityAt` set specifically on
  `StatusWaitingForAgent` transitions) — confirmed no existing field already
  serves this purpose — which is more than this task's ~4 min scope and is
  deferred as a follow-up rather than implemented here. Documented as a
  known limitation, not silently accepted: a session under active
  investigation (renamed, noted, tagged) while genuinely stuck in
  `StatusWaitingForAgent` can have its grace window reset by that
  investigation itself.
- Files: `session/review_queue_determiner.go`

##### Task 1.3.1b: Add controller-active `StatusWaitingForAgent` tests (~4 min)
- Add `TestDefaultStatusDeterminer_ControllerWaitingForAgent_RecentlyUpdated_Removed`
  and `TestDefaultStatusDeterminer_ControllerWaitingForAgent_Stale_FallsThroughToIdle`,
  mirroring `:678` and `:706` but with `statusInfo.IsControllerActive: true`.
- Files: `session/review_queue_determiner_test.go`

---

## Phase 2: Rule-Reconciliation

### Epic 2.1: Facade Wiring — `ApprovalService` Gains Internal Reconciliation Methods

**Goal**: `ApprovalService` becomes the single entry point for "resolve a
pending approval," whether the caller is a live human RPC or the
reconciliation pass, so every resolution path gets the CI-red guard,
metadata stamping, and event broadcast for free.

#### Story 2.1.1: `ApprovalService.ListPendingApprovalsInternal` and `ResolveApprovalReconciled`

**As a** developer wiring rule-reconciliation, **I want** one Go-level entry
point that resolves an approval exactly like a human's click would, plus a
distinguishable error when a human's click loses the race, **so that**
reconciliation never needs to duplicate `ResolveApproval`'s guard logic.

**Acceptance Criteria**:
- `ListPendingApprovalsInternal()` returns the same records
  `ListPendingApprovals` would, without proto marshaling.
  - *Given* two pending approvals in `ApprovalStore`, *When*
    `ListPendingApprovalsInternal()` is called, *Then* it returns
    `[]*PendingApproval` of length 2, matching `approvalStore.ListAll()`.
- `ResolveApprovalReconciled` resolves via the same path `ResolveApproval`
  uses, then stamps two extra metadata keys.
  - *Given* pending approval `appr-9f8e7d` for session `sess-a1b2c3`, *When*
    `ResolveApprovalReconciled(ctx, "appr-9f8e7d", "allow", "Auto-allow safe
    git status checks")` is called, *Then* the approval is removed from
    `ApprovalStore`, the notification record with `ID: "appr-9f8e7d"` has
    `metadata["approval_decision"] == "allow"` (from `ResolveApproval`'s
    existing stamp), `metadata["classifier_rule_name"] == "Auto-allow safe
    git status checks"`, and `metadata["reconciled"] == "true"`.
- A human's `ResolveApproval` call arriving after reconciliation already
  resolved the same approval gets a distinguishable error.
  - *Given* `appr-9f8e7d` was just resolved via
    `ResolveApprovalReconciled(..., "Auto-allow safe git status checks")`,
    *When* a client calls `ResolveApproval(ctx, {ApprovalId: "appr-9f8e7d",
    Decision: "deny"})`, *Then* the RPC returns `connect.CodeFailedPrecondition`
    with a message containing `"Auto-allow safe git status checks"` (not the
    generic `CodeNotFound` it returns for every other double-resolve case).
- **Human-vs-automation arbitration favors the human (research/ux.md: "favor
  the human, not the automation")**: once a live `ResolveApproval` RPC has
  begun processing an approval, a concurrent `ResolveApprovalReconciled`
  attempt on the same ID always backs off rather than racing
  `ApprovalStore`'s mutex, regardless of which one happens to reach the
  store first.
  - *Given* pending approval `appr-race` (`ToolInput: {"command": "rm -rf
    /tmp/scratch"}`), *When* a human's `ResolveApproval(ctx, {ApprovalId:
    "appr-race", Decision: "deny"})` call has started (has called
    `ApprovalStore.MarkHumanResolving`) but not yet returned, *And*
    `ResolveApprovalReconciled(ctx, "appr-race", "allow", "Auto-allow safe
    git status checks")` is called concurrently, *Then* the reconciliation
    call returns an error with `connect.CodeOf(err) ==
    connect.CodeAborted`, the approval is never resolved `"allow"`, and once
    the human's call completes, `appr-race` is resolved `"deny"`.

**Files**: `server/services/approval_service.go`, `server/services/approval_store.go`, `server/notifications/store.go`, `server/services/approval_handler.go`

##### Task 2.1.1a: Add `ListPendingApprovalsInternal` (~2 min)
- Add to `ApprovalService`:
  ```go
  // ListPendingApprovalsInternal returns all pending approvals for internal
  // Go callers (rule-reconciliation) that don't need ConnectRPC proto marshaling.
  func (as *ApprovalService) ListPendingApprovalsInternal() []*PendingApproval {
      return as.approvalStore.ListAll()
  }
  ```
- Files: `server/services/approval_service.go`

##### Task 2.1.1b: Add `GetByID` to `NotificationHistoryStore` and extend `approvalNotificationStamper` (~5 min)
- **Concurrency requirement (architecture-review.md Blocker 1)**: every other
  read-only method in this file (`List`, `GetUnreadCount`, `store.go:236-238`,
  `:430-432`) takes `s.mu.RLock()`, not `s.mu.Lock()` — match that convention.
  More importantly, `SetMetadata` (`store.go:411-424`, existing code) mutates
  a record's `Metadata` map in place under `s.mu.Lock()` from a different
  goroutine (the reconciliation pass calls it via
  `ResolveApprovalReconciled`, Task 2.1.1c). If `GetByID` returns the live
  `*NotificationRecord` pointer, a caller reading `rec.Metadata[...]` after
  the lock releases races with that in-place mutation — Go's "concurrent map
  read and map write" fatal error, not just a stale read. `GetByID` must
  therefore return a **deep copy** of the record (a copied `Metadata` map, at
  minimum a copied top-level struct) rather than the live pointer:
  ```go
  // GetByID returns a copy of the record with the given ID, if present. The
  // returned record (including its Metadata map) is safe to read after this
  // call returns without racing concurrent SetMetadata calls on the same ID —
  // it does not alias the store's live record or its Metadata map.
  func (s *NotificationHistoryStore) GetByID(id string) (*NotificationRecord, bool) {
      s.mu.RLock()
      defer s.mu.RUnlock()
      for _, r := range s.records {
          if r.ID == id {
              cp := *r
              if r.Metadata != nil {
                  cp.Metadata = make(map[string]string, len(r.Metadata))
                  for k, v := range r.Metadata {
                      cp.Metadata[k] = v
                  }
              }
              return &cp, true
          }
      }
      return nil, false
  }
  ```
- Add `GetByID(id string) (*NotificationRecord, bool)` to the
  `approvalNotificationStamper` interface (`approval_handler.go:52-55`);
  import `"github.com/tstapler/stapler-squad/server/notifications"` in
  `approval_handler.go` if not already present for the return type.
- **Update the two existing fakes that satisfy this interface structurally
  (architecture-review.md Concern — `approvalNotificationStamper` is shared
  by `ApprovalHandler.notificationStamper` and `ApprovalService.notificationStore`,
  not private to one call site) so `go build`/`go test` for `server/services`
  keeps compiling once `GetByID` is added:**
  - `spyStamper` (`approval_handler_integration_test.go:631-644`, passed to
    `h.SetNotificationStamper(spy)` at lines 747/812/854): add a fixed-stub
    `GetByID(id string) (*notifications.NotificationRecord, bool) { return
    nil, false }` — `ApprovalHandler` never calls `GetByID` itself (only
    `ApprovalService` does), so the stub only needs to satisfy the interface.
  - `spyNotificationStore` (`approval_service_test.go:449-463`, passed to
    `svc.SetNotificationStore(spy)` at line 472): add a **stateful**
    `GetByID` that returns a record reflecting the `SetMetadata` calls
    already recorded in its `callLog` (or a small settable
    `records map[string]*notifications.NotificationRecord` field updated
    from the existing `SetMetadata` stub) — a bare stub isn't enough here
    because Task 2.1.1d's
    `TestResolveApproval_AfterReconciled_ReturnsFailedPrecondition` needs
    `GetByID` to return a record whose `IsReconciled()` is actually `true`
    to exercise the not-found-but-reconciled branch at all.
- Files: `server/notifications/store.go`, `server/services/approval_handler.go`,
  `server/services/approval_handler_integration_test.go`,
  `server/services/approval_service_test.go`

##### Task 2.1.1c: Add `ResolveApprovalReconciled`, human-vs-reconciliation resolution arbitration, the `CodeFailedPrecondition`/`CodeAborted` race checks, and a shared `IsReconciled` accessor (~12 min)
- **Shared accessor (architecture-review.md Concern 3 / this plan's Concern 6)**:
  `metadata["reconciled"] == "true"` is checked at four call sites across Go
  and TS (this task's not-found branch, Task 2.3.2b's badge branch, and
  Task 3.1.3a's two filter changes) — a future typo at one write/read site
  (`"True"`, `" true"`) would silently break the others with no compiler
  help. Add one Go helper next to `NotificationRecord`'s definition
  (`store.go:34-58`) and use it at every Go call site instead of the inline
  comparison:
  ```go
  // IsReconciled reports whether this record's approval was resolved by
  // rule-reconciliation rather than a live human decision. Single source of
  // truth for the "reconciled" metadata key's value convention.
  func (r *NotificationRecord) IsReconciled() bool {
      return r.Metadata["reconciled"] == "true"
  }
  ```
- **Human-vs-automation arbitration (adversarial-review.md Blocker — "favor
  the human, not the automation" per research/ux.md:190-197)**. Root cause,
  verified against the actual current code: `ApprovalStore.Resolve()`
  (`approval_store.go:180-205`) is a plain `s.mu.Lock(); delete(s.pending,
  id); ...` with no notion of caller identity, and today's `ResolveApprovalReconciled`
  design routes reconciliation through the *same* `ResolveApproval` RPC
  method a human's click uses (`approval_service.go:61-145`) — but a human's
  decision reaches that method only after a ConnectRPC/HTTP round-trip,
  while reconciliation calls it directly in-process, so the CI-red-guard
  lookup at `approval_service.go:93-113` (which runs *before*
  `approvalStore.Resolve` at line 115) is a real latency window in which
  reconciliation can delete the pending entry before a human decision that
  had already reached the server gets a chance to. Fix: add an explicit,
  mutex-guarded "human resolution in flight" claim to `ApprovalStore` that a
  human's RPC sets as the *very first* thing it does — before the CI-guard
  lookup, before anything else — and that reconciliation must check before
  ever attempting to resolve. This narrows the CI-guard-lookup window — it
  does not close it entirely. As the residual-risk note on `resolveApproval`
  below documents (adversarial-review.md iteration 3), reconciliation checks
  `IsHumanResolving` once, at entry, and does not re-check immediately before
  `approvalStore.Resolve()` further down; a human's `MarkHumanResolving`
  landing after that single check but before reconciliation's `Resolve()`
  call still loses. So what's actually closed is the window between
  reconciliation *starting* and the CI-guard lookup completing (once a
  human's RPC handler has started running at all, its claim is set before
  any I/O) — the narrower "in-process CI-guard-lookup latency only" window is
  what remains, on top of the unavoidable, symmetric race no backend design
  can close (the network transport time for the human's click to reach the
  server in the first place).
  - Add to `ApprovalStore` (`approval_store.go`), guarded by the existing
    `s.mu`:
    ```go
    // humanResolving tracks approval IDs with a human-initiated resolution
    // currently in flight, so a concurrent rule-reconciliation pass can detect
    // and defer to it instead of racing Resolve()'s mutex (research/ux.md:
    // "favor the human, not the automation"). Guarded by mu, same as
    // pending/bySession.
    humanResolving map[string]struct{}
    ```
    Initialize it in `NewApprovalStore` alongside `pending`/`bySession`.
    ```go
    // MarkHumanResolving records that a human-initiated resolution is in
    // flight for id. Call as the very first step of the human resolution
    // path — before any other work, including the CI-red-guard lookup —
    // and always pair with a deferred ClearHumanResolving so the marker
    // cannot leak past one call.
    func (s *ApprovalStore) MarkHumanResolving(id string) {
        s.mu.Lock()
        defer s.mu.Unlock()
        s.humanResolving[id] = struct{}{}
    }

    // ClearHumanResolving removes the in-flight marker set by MarkHumanResolving.
    // Safe to call even if id was never marked (no-op).
    func (s *ApprovalStore) ClearHumanResolving(id string) {
        s.mu.Lock()
        defer s.mu.Unlock()
        delete(s.humanResolving, id)
    }

    // IsHumanResolving reports whether a human-initiated resolution is
    // currently in flight for id.
    func (s *ApprovalStore) IsHumanResolving(id string) bool {
        s.mu.RLock()
        defer s.mu.RUnlock()
        _, ok := s.humanResolving[id]
        return ok
    }
    ```
  - Refactor `ApprovalService.ResolveApproval` (`approval_service.go:60-145`)
    into a private core both the public RPC method and
    `ResolveApprovalReconciled` call, parameterized by who's calling:
    ```go
    type resolutionSource int

    const (
        resolutionSourceHuman resolutionSource = iota
        resolutionSourceReconciliation
    )

    // resolveApproval is the single internal entry point every resolution path
    // funnels through — a live human's RPC click and the rule-reconciliation
    // pass — so the CI-red guard, notification stamping, and event broadcast
    // apply uniformly, and the human/automation race (research/ux.md's "favor
    // the human, not the automation" mandate) is arbitrated in exactly one
    // place. Body is ResolveApproval's current logic (arg validation through
    // the final log line), unchanged except: (1) parameterized on approvalID/
    // decisionStr/message/overrideCIBlock instead of reading req.Msg directly,
    // and (2) the arbitration check below, added as the very first step.
    //
    // Residual risk (accepted, adversarial-review.md iteration 3): the
    // reconciliation branch checks IsHumanResolving once, at entry, and does
    // not re-check immediately before approvalStore.Resolve() further down.
    // A human's MarkHumanResolving landing after that single check but before
    // reconciliation's Resolve() call still loses. This narrows the race
    // window from iteration 2's "full RPC round-trip + guard lookup" down to
    // "in-process CI-guard-lookup latency only" — not zero, but small enough
    // that the reviewer did not classify it as a blocker for a single-operator
    // tool. Fully closing it would require moving the IsHumanResolving check
    // inside ApprovalStore.Resolve() itself, under the same lock as the
    // delete — deferred as a follow-up, not required for this project's scope.
    func (as *ApprovalService) resolveApproval(ctx context.Context, approvalID, decisionStr, message string, overrideCIBlock bool, source resolutionSource) (*sessionv1.ResolveApprovalResponse, error) {
        if approvalID == "" {
            return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("approval_id is required"))
        }
        if decisionStr != "allow" && decisionStr != "deny" {
            return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("decision must be 'allow' or 'deny'"))
        }

        if source == resolutionSourceHuman {
            // Claim before any other work — including the CI-guard lookup below,
            // which can block on a live-instance-registry read — so a human
            // decision that has already reached the server can never lose to
            // reconciliation's in-process call during that lookup.
            as.approvalStore.MarkHumanResolving(approvalID)
            defer as.approvalStore.ClearHumanResolving(approvalID)
        } else if as.approvalStore.IsHumanResolving(approvalID) {
            // A human decision is already in flight for this exact approval —
            // defer to it entirely rather than racing the store's mutex.
            return nil, connect.NewError(connect.CodeAborted,
                fmt.Errorf("approval %s has a human decision in flight", approvalID))
        }

        // ... unchanged: sessionID lookup, CI-red guard (using overrideCIBlock
        // in place of req.Msg.OverrideCiBlock), approvalStore.Resolve, the
        // IsReconciled()-aware not-found branch below, notification stamping,
        // event broadcast, and the final log line — all exactly as in the
        // current ResolveApproval body, just reading the function's
        // parameters instead of req.Msg. Returns *sessionv1.ResolveApprovalResponse
        // instead of *connect.Response[...] — the public wrapper below adds
        // the connect.Response envelope.
    }

    // ResolveApproval sends the user's decision to the blocked HTTP hook handler.
    func (as *ApprovalService) ResolveApproval(ctx context.Context, req *connect.Request[sessionv1.ResolveApprovalRequest]) (*connect.Response[sessionv1.ResolveApprovalResponse], error) {
        message := ""
        if req.Msg.Message != nil {
            message = *req.Msg.Message
        }
        resp, err := as.resolveApproval(ctx, req.Msg.ApprovalId, req.Msg.Decision, message, req.Msg.OverrideCiBlock, resolutionSourceHuman)
        if err != nil {
            return nil, err
        }
        return connect.NewResponse(resp), nil
    }

    // ResolveApprovalReconciled resolves a pending approval on behalf of the
    // rule-reconciliation pass (RulesService.reconcilePendingApprovals),
    // reusing resolveApproval's exact guard/arbitration/stamp/broadcast logic
    // so a reconciled resolution is indistinguishable in effect from a live
    // one — then stamps two extra metadata keys so the UI and a later human
    // click can both tell it apart from a live decision. Returns a
    // connect.CodeAborted error, without resolving anything, if a human
    // decision is currently in flight for approvalID (see resolveApproval).
    func (as *ApprovalService) ResolveApprovalReconciled(ctx context.Context, approvalID, decision, ruleName string) error {
        if _, err := as.resolveApproval(ctx, approvalID, decision, "", false, resolutionSourceReconciliation); err != nil {
            return err
        }
        if as.notificationStore != nil {
            _ = as.notificationStore.SetMetadata(approvalID, "classifier_rule_name", ruleName)
            _ = as.notificationStore.SetMetadata(approvalID, "reconciled", "true")
        }
        return nil
    }

    // IsApprovalPending reports whether approvalID is still present in the
    // pending store. Used by reconciliation to disambiguate a genuine
    // CI-red-guard decline (item remains pending) from "lost the race to a
    // concurrent reconciliation pass" (item already gone) when both surface
    // the same connect.CodeFailedPrecondition (adversarial-review.md Concern 2).
    func (as *ApprovalService) IsApprovalPending(approvalID string) bool {
        _, ok := as.approvalStore.Get(approvalID)
        return ok
    }
    ```
  - Within `resolveApproval`'s not-found branch (previously
    `ResolveApproval`'s, `approval_service.go:115-117`), check for the
    reconciled marker before returning the generic error, exactly as before.
    Per Task 2.1.1b's fix, `GetByID` now returns a **copy** of the record —
    safe to read here with no lock held:
    ```go
    if err := as.approvalStore.Resolve(approvalID, decision); err != nil {
        if as.notificationStore != nil {
            if rec, ok := as.notificationStore.GetByID(approvalID); ok && rec.IsReconciled() {
                ruleName := rec.Metadata["classifier_rule_name"]
                return nil, connect.NewError(connect.CodeFailedPrecondition,
                    fmt.Errorf("already auto-resolved by rule %q while you were reviewing it — no action needed", ruleName))
            }
        }
        return nil, connect.NewError(connect.CodeNotFound, err)
    }
    ```
- Files: `server/notifications/store.go`, `server/services/approval_service.go`, `server/services/approval_store.go`

##### Task 2.1.1d: Add tests for the new `ApprovalService` methods (~8 min)
- Add `TestResolveApprovalReconciled_StampsMetadata`,
  `TestResolveApproval_AfterReconciled_ReturnsFailedPrecondition` to the
  existing `server/services/approval_service_test.go`.
- **New, covering Task 2.1.1c's arbitration mechanism** (the real concurrent
  end-to-end race lives in Task 2.2.2c at the `RulesService` level — these
  are narrower unit tests pinning the primitives it depends on):
  - `TestApprovalStore_HumanResolving_MarksAndClears`: `MarkHumanResolving`
    then `IsHumanResolving` returns `true`; `ClearHumanResolving` then
    `IsHumanResolving` returns `false`; `ClearHumanResolving` on a
    never-marked ID is a no-op (no panic).
  - `TestResolveApprovalReconciled_DefersToInFlightHumanResolution`: call
    `approvalStore.MarkHumanResolving(id)` directly to simulate a human RPC
    already in flight, then call `ResolveApprovalReconciled(ctx, id, "allow",
    "some rule")`; assert `connect.CodeOf(err) == connect.CodeAborted` and
    the approval is still present in `ApprovalStore.ListAll()` afterward
    (reconciliation never touched it).
  - `TestIsApprovalPending_ReflectsStoreState`: `true` while pending, `false`
    after `Resolve`.
- Files: `server/services/approval_service_test.go`

#### Story 2.1.2: Wire `RulesService` to `ApprovalService`

**As a** developer, **I want** `RulesService` to hold a reference to
`ApprovalService`, **so that** the reconciliation pass has everything it
needs without a new dependency edge into `ApprovalStore` directly.

**Acceptance Criteria**:
- `RulesService.SetApprovalService` stores the reference; `nil` is the safe
  default (reconciliation no-ops if never wired, matching the
  `claudeSettingsWatcher` nil-safe pattern already used in this file).
- The wiring happens after both services are constructed, before either can
  receive traffic.
  - *Given* `session_service.go`'s existing construction order
    (`approvalSvc` at line ~666, `rulesSvc` at line ~721), *When*
    `NewSessionService` runs, *Then* `rulesSvc.SetApprovalService(approvalSvc)`
    is called before the function returns.

**Files**: `server/services/rules_service.go`, `server/services/session_service.go`

##### Task 2.1.2a: Add a narrow `reconciliationResolver` interface and wire `approvalSvc` against it (~3 min)
- **Interface, not concrete type (architecture-review.md Concern 1 / this
  plan's Concern 4)**: typing `RulesService.approvalSvc` as a concrete
  `*ApprovalService` forces every reconciliation-loop test (still-escalates,
  caps-at-max, decision mapping) to stand up a full `ApprovalStore` —
  confirmed by Task 2.2.2a's own instruction to seed a real-or-temp-file
  store. Task 2.3.1d's adjacent `ApprovalService → ReviewQueueRemover` edge
  already interfaces correctly; apply the same discipline one step earlier.
  Define, in `server/services/rules_service.go`:
  ```go
  // reconciliationResolver is the narrow contract reconcilePendingApprovals needs
  // from ApprovalService, satisfied implicitly by *ApprovalService. Letting
  // RulesService depend on this instead of the concrete type lets
  // reconciliation-loop tests inject a lightweight fake for cases that only
  // exercise classify-and-branch logic, without a real ApprovalStore.
  type reconciliationResolver interface {
      ListPendingApprovalsInternal() []*PendingApproval
      ResolveApprovalReconciled(ctx context.Context, approvalID, decision, ruleName string) error
      // IsApprovalPending disambiguates a genuine CI-red-guard decline from
      // "lost the race to a concurrent reconciliation pass" when
      // ResolveApprovalReconciled returns connect.CodeFailedPrecondition for
      // both (adversarial-review.md Concern 2) — see Task 2.2.1a.
      IsApprovalPending(approvalID string) bool
  }
  ```
- Add `approvalSvc reconciliationResolver` field (doc comment: "nil until
  `SetApprovalService` is called — `reconcilePendingApprovals` no-ops when
  nil, same idiom as `claudeSettingsWatcher`").
- Add `func (rs *RulesService) SetApprovalService(as reconciliationResolver) { rs.approvalSvc = as }`
  next to `SetClaudeSettingsWatcher` (`rules_service.go:58-63`) — accepting
  the interface (not `*ApprovalService`) so a test fake can be passed
  directly without an adapter.
- Files: `server/services/rules_service.go`

##### Task 2.1.2b: Wire the setter in `session_service.go` (~2 min)
- After `rulesSvc.SetClaudeSettingsWatcher(claudeSettingsWatcher)`
  (`session_service.go:740`), add `rulesSvc.SetApprovalService(approvalSvc)`.
  `approvalSvc` is already in scope from line 666.
- Files: `server/services/session_service.go`

---

### Epic 2.2: `reconcilePendingApprovals()` Core Loop

**Goal**: After any rule rebuild, pending `Escalate`-created approvals are
re-classified against the fresh rule set; any that now decide are resolved
through `ApprovalService` and logged, without blocking the RPC that
triggered the rebuild.

**Accepted limitation (pre-mortem.md #2)**: reconciliation only fires from
the two existing rebuild choke points — `UpsertApprovalRule`/
`DeleteApprovalRule`/`BulkUpsertRules` (Rules-UI edits) and
`ClaudeSettingsWatcher`'s reload of the *server process's own cwd*
`.claude/settings.json`/`.claude/settings.local.json` (per
`session_service.go`'s own comment: "Global-only for v1 — this is always the
server's own cwd, never a per-session git worktree path"). A rule added via
a session's per-worktree settings file — routine in this worktree-heavy
repo — never triggers `ClaudeSettingsWatcher` and therefore never triggers
reconciliation, so "adding or editing an approval rule auto-resolves any
currently-pending escalated approval" (Success Metrics) does not hold for
that path. Extending the watcher to every live worktree is a materially
larger change (dynamic per-session watch registration/teardown as sessions
come and go) than this Epic's Appetite covers, so it's scoped out rather
than folded in — Task 2.2.2h below pins the gap with a test instead of
leaving it to be discovered in production, and worktree coverage is a
candidate follow-up if it turns out to matter in practice.

#### Story 2.2.1: Implement and wire the reconciliation loop

**As a** user, **I want** adding a rule to auto-resolve every currently
pending approval it now covers, **so that** I stop having to manually click
through backlog that a new rule already makes obsolete.

**Acceptance Criteria**:
- A pending `Escalate` item that now classifies `AutoAllow` gets resolved.
  - *Given* pending approval `appr-1a2b3c` (`ToolName: "Bash"`, `ToolInput:
    {"command": "git status"}`, escalated because no rule matched at the
    time), *When* a new rule "Auto-allow safe git status checks" is added
    via `UpsertApprovalRule` and the classifier rebuild completes, *Then*
    `appr-1a2b3c` is resolved with decision `"allow"`, and its notification
    record carries `classifier_rule_name: "Auto-allow safe git status
    checks"` and `reconciled: "true"`.
- A pending item that still classifies `Escalate` is left untouched.
  - *Given* pending approval `appr-4d5e6f` (`ToolName: "Bash"`, `ToolInput:
    {"command": "rm -rf /tmp/scratch"}`), *When* the same rebuild runs,
    *Then* `appr-4d5e6f` is still present in `ApprovalStore.ListAll()`
    afterward.
- Reconciliation uses live-recomputed `SessionIdleMinutes`, not a stored
  value, since `PendingApproval` never persisted it.
- The RPC that triggered the rebuild (`UpsertApprovalRule`) returns before
  reconciliation of a large pending backlog completes (ADR-004).
- A panic anywhere in the reconciliation loop is recovered, logged, and
  never crashes the process — matching `ReviewQueuePoller.checkSessionsSafe`'s
  house idiom (adversarial-review.md Blocker 1).
- The pass-complete summary log's counts reflect real partial progress even
  when the pass panicked mid-loop — never zeroed out because the counters
  lived in a stack frame the panic unwound past (adversarial-review.md
  Concern, iteration 2).
- A CI-red-guard decline during reconciliation is logged and counted
  separately from (a) a resolved or generically-skipped item, and (b) a
  decline caused by losing a race to a *different* concurrent reconciliation
  pass — the two currently share one error code and must not be conflated
  (adversarial-review.md Concern 1 / Concern, iteration 2).
- Losing a race to an **in-flight human decision** (Task 2.1.1c's
  `connect.CodeAborted`) is logged and counted separately from all of the
  above, and never resolves the item. The human's decision wins whenever
  `MarkHumanResolving` lands before reconciliation's single `IsHumanResolving`
  check — a window substantially narrower than pre-fix, though not provably
  zero (see the accepted residual risk noted on Task 2.1.1c, adversarial-review.md
  iteration 3) — this is not an unconditional guarantee for every possible
  interleaving (adversarial-review.md Blocker, iteration 2; narrowed,
  iteration 3).

**Files**: `server/services/rules_service.go`

##### Task 2.2.1a: Implement `reconcilePendingApprovals()` with panic recovery (~10 min)
- **Panic recovery (adversarial-review.md Blocker 1)**: every comparable
  background goroutine in this codebase recovers from panics —
  `session/review_queue_poller.go`'s `checkSessionsSafe()` is the house
  idiom (`defer func() { if r := recover(); r != nil { ... log.Error(...) }
  }()`, confirmed at `review_queue_poller.go:408-418`), also used in
  `session_service.go`/`session_driver.go`. A bare `go
  rs.reconcilePendingApprovals()` has no such guard: a panic anywhere in the
  loop (e.g. a malformed custom rule, a nil-map access on `ToolInput`)
  crashes the entire process, not just the reconciliation pass.
- **Counters must survive a mid-loop panic (adversarial-review.md Concern,
  iteration 2)**: the previous draft of this task declared `resolved`/
  `skipped`/`declinedByGuard` as locals inside `reconcilePendingApprovals`
  itself, with the "pass complete" summary log at the end of that same
  function — but a panic partway through unwinds past both, and
  `reconcilePendingApprovalsSafe`'s `defer` (a *different* function's stack
  frame) has no access to those variables, so the doc comment's claim that
  partial counts survive a panic didn't match the code. Fix: declare the
  counters in the **outer** function (the one holding the `defer`/`recover`),
  as a small struct passed into the loop by pointer, and move the
  summary-log line into that same `defer` so it fires exactly once per pass
  — on the panic path and the normal path alike — reading the shared struct:
  ```go
  // reconcileCounts accumulates one pass's counters in reconcilePendingApprovalsSafe's
  // stack frame (not reconcilePendingApprovals'), so a mid-loop panic in the
  // latter never leaves the deferred summary log with stale/zeroed counts —
  // it reads the same struct the loop was just incrementing by pointer.
  type reconcileCounts struct {
      resolved, skipped, declinedByGuard, deferredToHuman, lostToConcurrentPass int
  }

  // reconcilePendingApprovalsSafe wraps reconcilePendingApprovals with panic
  // recovery, matching ReviewQueuePoller.checkSessionsSafe's house idiom for
  // background goroutines. The "pass complete" summary log lives here, not in
  // reconcilePendingApprovals, specifically so a mid-loop panic still logs
  // whatever partial progress counts had reached — the audit trail for a
  // pass must never disappear silently (Observability Requirement).
  func (rs *RulesService) reconcilePendingApprovalsSafe() {
      counts := &reconcileCounts{}
      defer func() {
          r := recover()
          if r != nil {
              log.Error("[RulesService] panic in reconcilePendingApprovals recovered", "panic", r,
                  "resolved_count", counts.resolved, "skipped_count", counts.skipped,
                  "declined_by_ci_guard_count", counts.declinedByGuard)
          }
          if r != nil || counts.resolved > 0 || counts.skipped > 0 || counts.declinedByGuard > 0 ||
              counts.deferredToHuman > 0 || counts.lostToConcurrentPass > 0 {
              log.Info("[RulesService] reconciliation pass complete",
                  "resolved_count", counts.resolved, "skipped_count", counts.skipped,
                  "declined_by_ci_guard_count", counts.declinedByGuard,
                  "deferred_to_human_count", counts.deferredToHuman,
                  "lost_to_concurrent_pass_count", counts.lostToConcurrentPass,
                  "capped", counts.skipped > 0, "panicked", r != nil)
          }
      }()
      rs.reconcilePendingApprovals(counts)
  }

  // reconcilePendingApprovals re-runs Classify() against every currently
  // pending, Escalate-sourced approval using the just-rebuilt rule set, and
  // resolves any that now decide via ApprovalService.ResolveApprovalReconciled.
  // counts is owned by the caller (reconcilePendingApprovalsSafe) and
  // incremented in place so its summary log survives a panic here. Called
  // after rebuildMu releases (see rebuildClassifier/rebuildClaudeSettingsRules) —
  // Classify() itself is a pure, side-effect-free read (pkg/classifier/classifier.go:456),
  // so this needs no lock of its own. Always call via reconcilePendingApprovalsSafe,
  // never directly, outside of tests that intentionally exercise the panic path.
  //
  // Accepted limitation (adversarial-review.md Minor 3): a pass reads
  // rs.classifier fresh on every item, with no rule-generation snapshot taken
  // at pass start. If a second rule edit lands while this pass is still
  // running (ADR-004 explicitly permits concurrent passes and takes no lock
  // here), later items in this same pass may be classified against a newer
  // ruleset than earlier items were, so one pass-complete summary log line
  // isn't always "one ruleset." Each item's own audit log line still
  // correctly attributes the rule that actually resolved it, so this doesn't
  // lose any per-item accuracy — only the pass-level summary's framing is
  // imprecise. Deliberately not guarded against: a generation-snapshot-and-skip
  // guard would need its own synchronization, which is exactly the
  // over-engineered-for-this-Appetite worker-queue complexity ADR-004
  // rejected, and at single-operator scale two rule edits landing within one
  // reconciliation pass's runtime is rare enough not to warrant it.
  func (rs *RulesService) reconcilePendingApprovals(counts *reconcileCounts) {
      if rs.approvalSvc == nil {
          return
      }
      // Deterministic, oldest-escalated-first order before the cap is applied
      // (pre-mortem.md #5): ApprovalStore.pending is a map, and
      // ListPendingApprovalsInternal/ListAll iterate it directly, so which N
      // of a >maxReconcileAutoResolvesPerPass backlog get resolved would
      // otherwise be arbitrary and different on every pass — not hypothetical,
      // this is the exact 107-item-backlog incident that triggered this
      // project. Sorting by CreatedAt (already recorded on every
      // PendingApproval, no new field needed) makes repeated passes monotonic:
      // the same oldest items resolve first every time, so a capped pass
      // followed by the next unrelated rule edit continues draining the
      // backlog in order instead of touching a random 50 each time.
      pending := rs.approvalSvc.ListPendingApprovalsInternal()
      sort.Slice(pending, func(i, j int) bool { return pending[i].CreatedAt.Before(pending[j].CreatedAt) })
      for _, a := range pending {
          payload := classifier.PermissionRequestPayload{
              ToolName: a.ToolName, ToolInput: a.ToolInput,
              Cwd: a.Cwd, PermissionMode: a.PermissionMode,
          }
          ctx := classifier.ClassificationContext{Cwd: a.Cwd}
          if rs.liveIdleMinutes != nil {
              ctx.SessionIdleMinutes = rs.liveIdleMinutes(a.SessionID)
          }
          result := rs.classifier.Classify(payload, ctx)
          if result.Decision == classifier.Escalate {
              continue
          }
          if counts.resolved >= maxReconcileAutoResolvesPerPass {
              counts.skipped++
              continue
          }
          decision := "deny"
          if result.Decision == classifier.AutoAllow {
              decision = "allow"
          }
          if err := rs.approvalSvc.ResolveApprovalReconciled(context.Background(), a.ID, decision, result.RuleName); err != nil {
              // Three-way classification (adversarial-review.md Blocker + Concern 1/2,
              // iteration 2). All three below can surface as errors from
              // ResolveApprovalReconciled, and must not be conflated:
              switch {
              case connect.CodeOf(err) == connect.CodeAborted:
                  // Lost the race to an in-flight HUMAN decision (Task 2.1.1c's
                  // arbitration) — research/ux.md's "favor the human" mandate in
                  // action, not an error. Never retried within this pass.
                  counts.deferredToHuman++
                  log.Info("[RulesService] reconciliation deferred to in-flight human decision",
                      "approval_id", a.ID, "session_id", a.SessionID)
              case connect.CodeOf(err) == connect.CodeFailedPrecondition:
                  // Both the CI-red guard (approval_service.go's block-on-red-CI check,
                  // *before* approvalStore.Resolve is reached) and "another reconciliation
                  // pass already resolved this" (Task 2.1.1c's IsReconciled()-aware
                  // not-found branch) surface this same code — disambiguate by checking
                  // whether the item is still present in the store (adversarial-review.md
                  // Concern 2): a real CI-guard decline leaves it pending; a lost race to a
                  // concurrent pass has already removed it.
                  if rs.approvalSvc.IsApprovalPending(a.ID) {
                      counts.declinedByGuard++
                      log.Info("[RulesService] reconciliation declined by CI-red guard",
                          "approval_id", a.ID, "session_id", a.SessionID,
                          "rule_id", result.RuleID, "rule_name", result.RuleName)
                  } else {
                      counts.lostToConcurrentPass++
                      log.Info("[RulesService] reconciliation lost race to a concurrent reconciliation pass",
                          "approval_id", a.ID, "session_id", a.SessionID)
                  }
              }
              continue // otherwise (e.g. CodeNotFound): lost the race to a human's
                       // already-completed, non-reconciled resolution — expected, uncounted
          }
          counts.resolved++
          log.Info("[RulesService] reconciled pending approval",
              "approval_id", a.ID, "session_id", a.SessionID,
              "rule_id", result.RuleID, "rule_name", result.RuleName,
              "before_decision", "escalate", "after_decision", decision)
      }
  }
  ```
- `rs.liveIdleMinutes` is a new optional `func(sessionID string) int` field
  (nil-safe) wired the same way as `approvalSvc` — deferred to Task 2.2.1d
  if a live-instance-lookup seam doesn't already exist on `RulesService`;
  if none is trivially available, ship this task with `SessionIdleMinutes`
  left at its zero value (documented fail-closed contract per
  `ClassificationContext.SessionIdleMinutes`'s existing doc comment) and
  note the gap in the task's own commit message rather than inventing a new
  live-instance dependency edge not already used elsewhere in this file.
- Add `const maxReconcileAutoResolvesPerPass = 50` near the top of the file.
- Add `"sort"` to this file's imports for the `sort.Slice` call above.
- Files: `server/services/rules_service.go`

##### Task 2.2.1b: Call `reconcilePendingApprovalsSafe()` asynchronously after `rebuildMu` releases (~4 min)
- Per ADR-004, change `rebuildClassifier()` (`:525-534`) from `defer
  rs.rebuildMu.Unlock()` to an explicit unlock before spawning the
  reconciliation goroutine. Spawn the **panic-recovering wrapper**
  (`reconcilePendingApprovalsSafe`, Task 2.2.1a), never the bare method —
  a bare `go rs.reconcilePendingApprovals()` here would defeat the whole
  point of Task 2.2.1a's recovery wrapper:
  ```go
  func (rs *RulesService) rebuildClassifier() {
      rs.rebuildMu.Lock()
      userRules := rs.rulesStore.ToRules()
      existing := rs.classifier.Rules()
      rs.afterRebuildReadHook()
      nonUser := filterRulesBySource(existing, classifier.SourceSeed, classifier.SourceClaudeSettings)
      rs.classifier.ReplaceRules(append(nonUser, userRules...))
      rs.rebuildMu.Unlock()
      go rs.reconcilePendingApprovalsSafe()
  }
  ```
- Apply the identical change to `rebuildClaudeSettingsRules()` (`:539-547`).
- Files: `server/services/rules_service.go`

##### Task 2.2.1c: Add a test-only synchronization hook for the async reconciliation call (~3 min)
- `rebuildClassifier`/`rebuildClaudeSettingsRules` already have `testHook`
  for the rebuild's own critical section (`afterRebuildReadHook`). Add a
  second nil-safe hook, `reconcileDoneHook func()`. Call it last inside
  `reconcilePendingApprovalsSafe`'s `defer` — after Task 2.2.1a's
  `recover()`-and-summary-log logic, not inside `reconcilePendingApprovals()`
  itself — so it fires exactly once per pass whether or not the pass
  panicked, letting
  `TestReconcilePendingApprovals_PanicIsRecovered` (Task 2.2.2e) observe
  completion the same way the happy-path tests do, via `<-doneCh` instead of
  sleeping (matches this file's own documented reasoning for why `testHook`
  exists: "a bug here is a lost-update race, not a data race — `go test
  -race` cannot detect it").
- Files: `server/services/rules_service.go`

#### Story 2.2.2: Tests for the reconciliation loop

**Acceptance Criteria**: covered by Task-level tests below; no additional
criteria beyond Story 2.2.1's.

**Files**: `server/services/rules_service_test.go`

##### Task 2.2.2a: `TestReconcilePendingApprovals_AutoAllowResolves` (~5 min)
- Now that `RulesService.approvalSvc` is typed against the
  `reconciliationResolver` interface (Task 2.1.2a), the classify-and-branch
  logic (resolved vs. still-escalates vs. capped) can be exercised with a
  lightweight in-memory fake implementing `ListPendingApprovalsInternal`/
  `ResolveApprovalReconciled`/`IsApprovalPending` — no real `ApprovalStore`
  needed for those cases (`IsApprovalPending` can be a simple field- or
  map-backed stub the test configures per case; it only needs to be
  meaningful for Task 2.2.2f/2.2.2g's CI-guard-vs-concurrent-pass
  disambiguation). Reserve the real `ApprovalService` + `ApprovalStore` (temp
  file, matching this test file's existing fixture pattern) for the smaller
  set of tests that need to assert against real persisted state end-to-end
  (this one, since it also asserts the notification record's `reconciled:
  "true"` stamp via the real `NotificationHistoryStore`).
- Seed one `Escalate`-created pending approval, add a rule via
  `UpsertApprovalRule` that would now `AutoAllow` it, wait on
  `reconcileDoneHook`, assert the approval is gone from `ListAll()` and the
  notification record carries `reconciled: "true"`.
- Files: `server/services/rules_service_test.go`

##### Task 2.2.2b: `TestReconcilePendingApprovals_StillEscalates_LeftUntouched` (~3 min)
- Use the lightweight fake `reconciliationResolver` (Task 2.2.2a) — this
  case only needs to assert the loop's branching, not real persistence. The
  new rule doesn't match the pending item's tool call; assert
  `ResolveApprovalReconciled` is never called on the fake and the item
  remains unresolved.
- Files: `server/services/rules_service_test.go`

##### Task 2.2.2c: `TestReconcilePendingApprovals_HumanWinsRace` (~10 min)
- **Real concurrent race, not a sequential simulation** (adversarial-review.md
  Blocker, iteration 2: the previous draft of this test resolved the
  approval via a human call *before* triggering reconciliation — sequential
  ordering that never exercises the arbitration mechanism at all). Needs the
  real `ApprovalService` + `ApprovalStore` fixture (Task 2.2.2a's pattern),
  since it exercises `ApprovalStore.MarkHumanResolving`/`IsHumanResolving`
  directly, not just `RulesService`'s branching logic.
  - Seed pending approval `appr-race` (`ToolInput: {"command": "rm -rf
    /tmp/scratch"}`) and a rule that would `AutoAllow` it. Keep the
    `*PendingApproval` value returned/passed to `store.Create` in scope — its
    unexported `decisionCh` field (same package, directly readable from the
    test) is the ground truth for which decision actually got delivered,
    independent of any higher-level stamping.
  - Launch a goroutine simulating a human's in-flight `ResolveApproval` call
    *at the `ApprovalStore` primitive level* (the exact two calls
    `resolveApproval`'s human branch makes, deliberately isolated from the
    CI-guard/stamping/broadcast machinery around them so the test's sync
    point lands exactly on the arbitration boundary, not somewhere inside
    unrelated I/O): call `approvalStore.MarkHumanResolving("appr-race")`,
    close a `humanClaimed chan struct{}` to signal the claim is set, block on
    a second channel `releaseHuman`, then call
    `approvalStore.Resolve("appr-race", ApprovalDecision{Behavior: "deny",
    Message: "blocking this"})` and `approvalStore.ClearHumanResolving("appr-race")`.
  - On the test's main goroutine, `<-humanClaimed` (guarantees
    `MarkHumanResolving` happens-before the next step — this is the sync
    point the Blocker asked for, not a hope-it-interleaves-right sleep), then
    call `rulesSvc.approvalSvc.ResolveApprovalReconciled(ctx, "appr-race",
    "allow", "Auto-allow safe git status checks")` directly; assert
    `connect.CodeOf(err) == connect.CodeAborted` and `appr-race` is still
    present in `approvalStore.ListAll()` (reconciliation never touched it).
  - Close `releaseHuman`, wait for the goroutine, then assert: no panic;
    `appr-race` is no longer in `approvalStore.ListAll()` (resolved); and —
    the decisive assertion — reading `approval.decisionCh` (non-blocking,
    buffered capacity 1, already delivered by this point) yields
    `ApprovalDecision{Behavior: "deny", Message: "blocking this"}`, i.e. the
    human's decision, never the rule's `"allow"`. This is what "the human's
    resolution always wins" concretely means at the mechanism level, not an
    inference from error codes alone.
  - Also run the same scenario through the full `reconcilePendingApprovals`
    loop (not just a direct `ResolveApprovalReconciled` call) to confirm
    `counts.deferredToHuman` increments and the
    `"reconciliation deferred to in-flight human decision"` log line fires.
- Files: `server/services/rules_service_test.go`

##### Task 2.2.2d: `TestReconcilePendingApprovals_CapsAtMaxPerPass` (~4 min)
- Use the lightweight fake `reconciliationResolver` (Task 2.2.2a) — capping
  is pure loop-counting logic, no real store needed. Seed
  `maxReconcileAutoResolvesPerPass + 5` pending items all matching the new
  rule; assert exactly `maxReconcileAutoResolvesPerPass` are resolved
  and the "capped" summary log fires (verify via a captured log sink or the
  `skipped_count` reasoning, whichever this repo's existing log-testing
  convention uses — check `rules_service_test.go` for an existing log
  capture helper before adding a new one).
- **Deterministic-order assertion (pre-mortem.md #5)**: seed the fake's
  `ListPendingApprovalsInternal()` items with distinct, deliberately
  out-of-map-insertion-order `CreatedAt` values, run two passes back to back
  (second pass's fake returns whatever the first pass left unresolved), and
  assert the *same* oldest-first set resolves on pass one and the pass-two
  set is exactly the remainder — pinning that the sort in
  `reconcilePendingApprovals` (this task's parent, Task 2.2.1a) makes
  repeated capped passes monotonic instead of touching an arbitrary subset
  each time.
- Files: `server/services/rules_service_test.go`

##### Task 2.2.2e: `TestReconcilePendingApprovals_PanicIsRecovered` (~4 min)
- Covers adversarial-review.md Blocker 1's recovery wrapper. Inject a fake
  `reconciliationResolver` whose `ListPendingApprovalsInternal()` returns an
  item that causes `Classify()` (or the fake itself) to panic partway
  through a multi-item batch. Call `reconcilePendingApprovalsSafe()`
  directly, wait on `reconcileDoneHook`, and assert: (1) the call returns
  normally (no propagated panic — process doesn't crash), (2) a
  `log.Error("panic in reconcilePendingApprovals recovered", ...)` line
  fires, and (3) items enumerated *before* the panicking one were still
  resolved, **and** the `"reconciliation pass complete"` summary log also
  fires (Task 2.2.1a's fix — the counters live in
  `reconcilePendingApprovalsSafe`'s frame, not the panicking function's, so
  they survive) with `resolved_count` reflecting exactly those pre-panic
  resolutions and `"panicked", true` (captured via the same log-testing
  convention as Task 2.2.2d).
- Files: `server/services/rules_service_test.go`

##### Task 2.2.2f: `TestReconcilePendingApprovals_CIRedGuardDecline_LoggedAndCountedSeparately` (~5 min)
- Covers adversarial-review.md Concern 1 (CI-red-guard declines silently
  uncounted) — renamed from the stale `_LoggedAndUncounted` name, which
  described the pre-fix bug this test actually verifies is fixed. Seed a
  pending `Escalate` item whose session has a failing-CI branch (matching
  the fixture pattern `ApprovalService`'s existing block-on-red-CI tests
  use) and a new rule that would `AutoAllow` it; assert
  `ResolveApprovalReconciled` returns `CodeFailedPrecondition`, `rs.approvalSvc.IsApprovalPending`
  (or the real store's `Get`) still returns `true` for it, the item is
  neither resolved nor generically skipped, the `"reconciliation declined by
  CI-red guard"` log line fires with the approval/session/rule IDs, and the
  pass-complete summary's `declined_by_ci_guard_count` reflects it while
  `lost_to_concurrent_pass_count` stays `0` — this is the counterpart
  assertion to Task 2.2.2g's, confirming the two `CodeFailedPrecondition`
  causes are actually disambiguated, not just both counted somewhere.
- Files: `server/services/rules_service_test.go`

##### Task 2.2.2g: `TestReconcilePendingApprovals_LostToConcurrentPass_CountedSeparatelyFromCIGuard` (~5 min)
- Covers adversarial-review.md Concern 2 (iteration 2): a decline caused by
  losing a race to a *different* concurrent reconciliation pass returns the
  same `connect.CodeFailedPrecondition` as a genuine CI-red-guard decline,
  and must not be mislabeled as one. Use the **lightweight fake**
  `reconciliationResolver` (Task 2.2.2a's pattern) rather than the real
  store — this needs to simulate a stale snapshot: a *real* store's
  `ListPendingApprovalsInternal()` would simply omit an item another pass
  already resolved, so a real-store version of this test could never reach
  the loop body under test at all. Configure the fake so:
  `ListPendingApprovalsInternal()` returns the one pending item (as if this
  pass's snapshot was taken *before* the concurrent pass resolved it);
  `ResolveApprovalReconciled(...)` returns
  `connect.NewError(connect.CodeFailedPrecondition, ...)` (as `resolveApproval`'s
  `IsReconciled()`-aware not-found branch would once the item is actually
  gone); `IsApprovalPending(id)` returns `false` (the concurrent pass already
  removed it from the real store by the time this pass gets to it). Assert
  the `"reconciliation lost race to a concurrent reconciliation pass"` log
  line fires (not `"...declined by CI-red guard"`), and the pass-complete
  summary's `lost_to_concurrent_pass_count` reflects it while
  `declined_by_ci_guard_count` stays `0` — the counterpart assertion to Task
  2.2.2f's, confirming the disambiguation branches both ways.
- Files: `server/services/rules_service_test.go`

##### Task 2.2.2h: `TestReconcilePendingApprovals_WorktreeSettingsChange_DoesNotTrigger` (~3 min)
- Pins the accepted limitation documented on Epic 2.2's Goal (pre-mortem.md
  #2): a settings change confined to a session's per-worktree
  `.claude/settings.json`/`.claude/settings.local.json` does not go through
  `ClaudeSettingsWatcher` (which only watches the server process's own cwd)
  and therefore never calls `rebuildClaudeSettingsRules()` /
  `reconcilePendingApprovalsSafe()`. Simulate this directly at the
  `RulesService` level (no filesystem watcher needed): seed a pending
  `Escalate` item that a rule would now cover, add that rule via a path that
  mimics a worktree-scoped settings write (i.e., anything other than
  `UpsertApprovalRule`/`rebuildClaudeSettingsRules`), and assert
  reconciliation never runs and the item is still pending — documenting the
  gap as a test instead of leaving it to be discovered in production.
- Files: `server/services/rules_service_test.go`

---

### Epic 2.3: Mid-Review-Race UX

**Goal**: A user who has a pending-approval item open (either on the
Notifications page or in the Review Queue panel) when reconciliation
resolves it out from under them sees a visible, attributable state change —
never a silent disappearance or a generic error.

#### Story 2.3.1: Backend — carry the rule name through review-queue removal

**As a** developer, **I want** the review queue's removal event to
optionally carry which rule auto-resolved the item, **so that** the panel
can show *why* an item vanished instead of just that it did.

**Acceptance Criteria**:
- A normal (human-driven) removal still reports `reason: "user_action"`
  with no rule name, unchanged from today.
- A reconciliation-driven removal reports the rule name.
  - *Given* `ApprovalService.ResolveApprovalReconciled` resolves
    `appr-9f8e7d` for session `sess-a1b2c3` with rule name "Auto-allow safe
    git status checks", *When* the review queue's corresponding item is
    removed, *Then* the `WatchReviewQueue` client receives an `item_removed`
    event with `session_id: "sess-a1b2c3"`, `reason:
    "auto_resolved_by_rule"`, `auto_resolved_by_rule: "Auto-allow safe git
    status checks"`.

**Files**: `proto/session/v1/events.proto`, `session/queue/queue.go`, `session/review_queue.go`, `server/review_queue_manager.go`, `session/review_queue_test.go`, `server/services/approval_service.go`, `server/services/session_service.go`

##### Task 2.3.1a: Add the proto field (~2 min)
- In `ReviewQueueItemRemovedEvent` (`events.proto:419-426`), add:
  ```proto
  // Set only when reason == "auto_resolved_by_rule": the display name of
  // the rule that resolved the underlying approval. Empty for every other
  // removal reason.
  optional string auto_resolved_by_rule = 3;
  ```
- Run `make proto-gen`; do not commit generated output per this repo's
  `.gitignore` policy (commit only the `.proto` change).
- Files: `proto/session/v1/events.proto`

##### Task 2.3.1b: Add `RemovalInfo` (with smart constructors, not a bare struct) and `RemoveWithInfo` to the queue package (~6 min)
- **Make the illegal state unrepresentable (architecture-review.md Concern
  2 / this plan's Concern 5)**: a bare `RemovalInfo{Reason, RuleName}`
  struct lets a caller construct `RemovalInfo{Reason: "user_action",
  RuleName: "some-rule"}` or `RemovalInfo{Reason: "auto_resolved_by_rule"}`
  with an empty `RuleName` — the doc comment's pairing invariant would be
  comment-enforced only. With only two call sites today (`Remove()` and
  Task 2.3.1d's reconciliation call), unexported fields plus two smart
  constructors are cheap and close the gap without a full sum type. In
  `session/queue/queue.go`, add:
  ```go
  // RemovalInfo carries why a review item was removed, beyond the plain
  // sessionID Remove() gives observers today. Constructed only via
  // UserActionRemoval() or AutoResolvedByRuleRemoval(ruleName) so a
  // Reason/RuleName mismatch (e.g. "user_action" paired with a non-empty
  // RuleName) is not representable.
  type RemovalInfo struct {
      reason   string
      ruleName string
  }

  func (r RemovalInfo) Reason() string   { return r.reason }
  func (r RemovalInfo) RuleName() string { return r.ruleName }

  // UserActionRemoval is the RemovalInfo for a normal, human-driven removal.
  func UserActionRemoval() RemovalInfo {
      return RemovalInfo{reason: "user_action"}
  }

  // AutoResolvedByRuleRemoval is the RemovalInfo for a removal driven by
  // rule-reconciliation resolving the underlying approval. ruleName must be
  // non-empty — it is always available at this call site (Task 2.3.1d).
  func AutoResolvedByRuleRemoval(ruleName string) RemovalInfo {
      return RemovalInfo{reason: "auto_resolved_by_rule", ruleName: ruleName}
  }
  ```
- Change `ReviewQueueObserver.OnItemRemoved(sessionID string)` to
  `OnItemRemoved(sessionID string, info RemovalInfo)` (`:196`).
- Add `func (rq *ReviewQueue) RemoveWithInfo(sessionID string, info
  RemovalInfo) bool`, moving `Remove`'s existing body into it, then change
  `Remove(sessionID string) bool` to `return rq.RemoveWithInfo(sessionID,
  UserActionRemoval())` — every existing ~10 call sites of `Remove()` are
  unaffected.
- Update the observer-notification call at the end of `RemoveWithInfo`
  (was `observer.OnItemRemoved(sessionID)`) to `observer.OnItemRemoved(sessionID,
  info)`.
- Downstream consumers read via the accessor methods (`info.Reason()`,
  `info.RuleName()`), not struct-literal field access — update Task 2.3.1c's
  `ReactiveQueueManager` wiring and Task 2.3.1d's caller accordingly.
- Files: `session/queue/queue.go`

##### Task 2.3.1c: Update the re-export, `ReactiveQueueManager`, and the test double (~5 min)
- **File correction (architecture-review.md Concern 4 / this plan's Concern
  7)**: the original task instructed defining `ReviewQueueRemover` "in
  `session/queue/queue.go` alongside `ReviewQueueWriter`" — but
  `ReviewQueueWriter` is not defined in `queue.go` at all; it's hand-written
  directly in `session/review_queue.go:60-64`. Every existing
  single-method writer/observer-style interface for this type follows that
  placement convention. Define `ReviewQueueRemover` there instead, against
  `*ReviewQueue` directly (no alias-back needed):
  ```go
  // ReviewQueueRemover is the removal-side interface for the review queue,
  // matching ReviewQueueWriter's placement/convention above. Satisfied by
  // *ReviewQueue.
  type ReviewQueueRemover interface {
      RemoveWithInfo(sessionID string, info RemovalInfo) bool
  }
  ```
- `session/review_queue.go`: also add `type RemovalInfo = queue.RemovalInfo`
  and re-export the two smart constructors from Task 2.3.1b so callers in
  `server/services` (which imports `session`, not `session/queue`) can build
  one: `var UserActionRemoval = queue.UserActionRemoval` and `var
  AutoResolvedByRuleRemoval = queue.AutoResolvedByRuleRemoval`.
- `server/review_queue_manager.go`: change
  `OnItemRemoved(sessionID string)` to
  `OnItemRemoved(sessionID string, info session.RemovalInfo)`, set `Reason:
  info.Reason()` and, if `info.RuleName() != ""`, `AutoResolvedByRule:
  &ruleName` (copy `info.RuleName()` into a local first — taking `&` of a
  method return value doesn't compile) on the proto event.
- `session/review_queue_test.go`: update `testObserver.OnItemRemoved`
  (`:634`) to the new two-arg signature.
- Files: `session/review_queue.go`, `server/review_queue_manager.go`, `session/review_queue_test.go`

##### Task 2.3.1d: Wire `ApprovalService` to remove the review-queue item on reconciled resolve (~4 min)
- Add `reviewQueueRemover session.ReviewQueueRemover` field (nil-safe) and
  `SetReviewQueueRemover` setter to `ApprovalService`.
- In `ResolveApprovalReconciled` (Task 2.1.1c), after the metadata stamps,
  add: `if as.reviewQueueRemover != nil { as.reviewQueueRemover.RemoveWithInfo(sessionID,
  session.AutoResolvedByRuleRemoval(ruleName)) }` — using the smart
  constructor (Task 2.3.1b), not a struct literal, so the Reason/RuleName
  pairing can't drift. This is a no-op (`false`, no observer call) if the
  poller already removed it first, matching `Remove()`'s existing idempotent
  semantics.
- In `session_service.go`, add `approvalSvc.SetReviewQueueRemover(reviewQueue)`
  after `reviewQueue := session.NewReviewQueue()` (line 643) and
  `approvalSvc := NewApprovalService(approvalStore)` (line 666) are both in
  scope.
- Files: `server/services/approval_service.go`, `server/services/session_service.go`

##### Task 2.3.1e: Tests (~4 min)
- Add `TestReviewQueue_RemoveWithInfo_CarriesRuleName` (constructing via
  `AutoResolvedByRuleRemoval("some rule")` and asserting
  `info.Reason()`/`info.RuleName()` on the observer callback) and
  `TestReviewQueue_Remove_StillReportsUserAction` (asserting `Remove()`
  still reaches the observer with `UserActionRemoval()`'s values) in
  `session/review_queue_test.go`.
- Files: `session/review_queue_test.go`

#### Story 2.3.2: Frontend — distinct labeling on both approval-card surfaces

**As a** user, **I want** to immediately see when a rule auto-resolved an
approval I was looking at (whether I'd already clicked or not), **so that**
I never wonder whether the item silently vanished or something went wrong.

**Acceptance Criteria**:
- Notifications page: a reconciled approval renders a distinct badge, not
  the plain "✓ Approved" a live decision gets.
  - *Given* a notification with `metadata: {approval_decision: "allow",
    reconciled: "true", classifier_rule_name: "Auto-allow safe git status
    checks"}`, *When* `NotificationItem` renders its resolved-approval
    branch, *Then* it shows "✓ Auto-resolved by rule: Auto-allow safe git
    status checks" instead of "✓ Approved".
- Notifications page: a human's click that loses the race shows the
  specific message, not a generic "Expired" badge.
  - *Given* `resolveApproval` throws a `ConnectError` with
    `code === Code.FailedPrecondition` and message containing "auto-resolved
    by rule", *When* the catch handler in `useApprovalResolution.ts` runs,
    *Then* `blockedApprovals[approvalId]` is set to that message (reusing
    the existing CI-block inline-message UI slot) instead of falling into
    the generic `catch` branch that sets `"expired"`.
- Review Queue panel: an item reconciled while open is disabled in place
  with an explanatory banner, not abruptly removed (ux.md's "disable, don't
  hide" spec).
  - *Given* a `WatchReviewQueue` `item_removed` event with
    `auto_resolved_by_rule: "Auto-allow safe git status checks"` for session
    `sess-a1b2c3`, *When* `useReviewQueue`'s event handler processes it,
    *Then* the row for `sess-a1b2c3` stays visible but disabled (its
    approve/deny actions disabled), showing "Auto-resolved by rule:
    Auto-allow safe git status checks" with a "why" link to the rule, for
    ~5 seconds, before the row is finally removed from view.

**Files**: `web-app/src/lib/hooks/useApprovalResolution.ts`, `web-app/src/components/ui/NotificationItem.tsx`, `web-app/src/lib/hooks/useReviewQueue.ts`, `web-app/src/components/sessions/ReviewQueuePanel.tsx`, `web-app/src/lib/utils/notificationMapping.ts`

##### Task 2.3.2a: `useApprovalResolution.ts` — route `FailedPrecondition` into `blockedApprovals` regardless of message shape (~3 min)
- The existing `catch` block (`:89-92`) already checks
  `err.code === Code.FailedPrecondition` and sets `blockedApprovals` — this
  already works for the new reconciliation-race error since it's also
  `CodeFailedPrecondition`. Verify no CI-specific assumption leaks in
  (there is none in the current code) and add a unit test asserting a
  non-CI `FailedPrecondition` message also lands in `blockedApprovals`. No
  test file exists yet for this hook (`ls
  web-app/src/lib/hooks/useApprovalResolution.test.ts` confirms absent) —
  create one.
- Files: `web-app/src/lib/hooks/useApprovalResolution.test.ts` (new)

##### Task 2.3.2b: `NotificationItem.tsx` — distinct resolved-badge copy for reconciled items (~5 min)
- **Shared TS accessor (architecture-review.md Concern 3 / this plan's
  Concern 6)**: add one helper to `notificationMapping.ts` (alongside the
  Go `IsReconciled()` from Task 2.1.1c) and use it at this call site and
  both of Task 3.1.3a's filter changes, instead of three independent
  `metadata?.reconciled === "true"` inline comparisons:
  ```ts
  export function isReconciledNotification(n: { metadata?: Record<string, string> }): boolean {
    return n.metadata?.reconciled === "true";
  }
  ```
- In the `resolvedApprovals[approvalId]` branch (`:209-211`), check
  `isReconciledNotification(notification)` before the plain badges:
  ```tsx
  if (resolved === "allow" || resolved === "deny") {
    if (isReconciledNotification(notification)) {
      const ruleName = notification.metadata?.classifier_rule_name ?? "a rule";
      return <span className={resolvedBadge} data-decision={`${resolved}-reconciled`}
             title="Re-evaluated with current context (e.g. CI status, session idle time) at resolution time, which may have changed since this was first escalated.">
        {resolved === "allow" ? "✓" : "✗"} Auto-resolved by rule: {ruleName}
      </span>;
    }
    return <span className={resolvedBadge} data-decision={resolved}>
      {resolved === "allow" ? "✓ Approved" : "✗ Denied"}
    </span>;
  }
  ```
- **Attribution isn't perfectly precise — say so (adversarial-review.md
  Minor 2 / pitfalls.md's dropped caveat)**: `reconcilePendingApprovals()`
  re-runs `Classify()` with a *freshly gathered* `ClassificationContext`
  (current CI status, current session idle minutes), not the context that
  existed at the original escalation time (pitfalls.md, "Stale
  command/classification context"). A pending item can therefore flip to
  auto-decide for a reason only incidentally related to the rule that was
  just added/edited — e.g. CI went green in the interim. pitfalls.md
  explicitly asks the plan to "acknowledge this attribution isn't perfectly
  precise rather than presenting it as more certain than it is"; that
  caveat existed in research but was dropped from this task's earlier
  draft. The `title` tooltip above restores it at low cost (a hover, not
  extra UI chrome) rather than adding a second visible disclaimer line.
- In the `blockedMessage` branch (`:212-240`), skip rendering the
  "Approve anyway"/"View CI run" override actions when the message doesn't
  contain a CI-checks URL (`splitCIBlockMessage`'s `checksUrl` is already
  `undefined` for a reconciliation message — add a guard so only the "Deny"
  action, or no actions at all, render for that case, since there is
  nothing left to "approve anyway" against).
- Files: `web-app/src/components/ui/NotificationItem.tsx`,
  `web-app/src/lib/utils/notificationMapping.ts`

##### Task 2.3.2c: `useReviewQueue.ts` / `ReviewQueuePanel.tsx` — disable the row in place, don't remove it immediately (~6 min)
- **Match ux.md's literal spec (adversarial-review.md Concern 4)**: ux.md
  (research/ux.md:179-184) specifies that an item reconciled while open
  should show an inline banner *on that item* and disable (not hide) its
  action buttons, replaced with a "why" link/tooltip pointing at the rule —
  "the system's decision is visible and explained, not just enforced." The
  original task instead dispatched `removeItem(sessionId)` immediately and
  rendered a transient banner "above where its row used to be" — the row
  itself vanished, which risks a stray click landing on nothing if the user
  was mid-interaction, and doesn't match the spec's disable-in-place intent.
- In the `itemRemoved` case (`useReviewQueue.ts:247-250`), when
  `event.event.value.autoResolvedByRule` is set: do **not** dispatch
  `removeItem(sessionId)` immediately. Instead invoke an optional
  `onAutoResolved?(sessionId, ruleName)` callback, and defer the actual
  `removeItem(sessionId)` dispatch until after the disabled-row display
  window (~5s) elapses — matching the plain `itemRemoved` path's eventual
  removal, just delayed for this one case so the disabled state is visible
  first.
- In `ReviewQueuePanel.tsx`, keep a local `useState<Record<string, string>>`
  map of session → rule name, populated by that callback. For any row whose
  session is present in the map: render it disabled (dim the row, disable
  its approve/deny action buttons) with an inline banner/tooltip reading
  "Auto-resolved by rule: `<name>` — no action needed" and a "why" link/
  tooltip pointing at the rule, instead of removing the row from the list.
  Clear the map entry (and let the deferred `removeItem` take effect) after
  ~5s via a `setTimeout`.
- **Multiple simultaneous banners** (design/ux.md Surface 9, AC 22): the
  `Record<string, string>` map above is already keyed per session, so N
  concurrent reconciliations from one rule-reload pass each get their own
  entry, their own row-level banner, and their own independent `setTimeout` —
  no additional design is needed for this case, it falls out of the map
  being keyed by session rather than holding a single global banner value.
- **Focus management on the deferred removal** (design/ux.md Surface 9, AC
  33): when the deferred `removeItem(sessionId)` finally fires and a row
  that was keyboard-focused disappears, move focus to the next remaining
  row (by list index) or to the list's container/heading if it was the last
  row — mirroring the same pattern `NeedsDecisionSection` needs for its own
  item removals (Task 3.1.2b).
- Files: `web-app/src/lib/hooks/useReviewQueue.ts`, `web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 2.3.2d: Remaining frontend tests (~5 min)
- Task 2.3.2a already covers the `useApprovalResolution.test.ts`
  `FailedPrecondition` case. Add here: a `NotificationItem.test.tsx` case
  asserting the "Auto-resolved by rule: ..." badge renders when
  `metadata.reconciled === "true"` (and the plain "✓ Approved" still
  renders when it isn't), and a `ReviewQueuePanel.test.tsx` case asserting
  that after an `item_removed` event carrying `autoResolvedByRule`, the row
  stays present-but-disabled with the explanatory banner for the ~5s window,
  and is only removed from the list after that window elapses.
- Files: `web-app/src/components/ui/NotificationItem.test.tsx` (or its
  existing `__tests__/` location — check first), `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx`

---

## Phase 3: Frontend IA Reboot

### Epic 3.1: Notifications Page IA

#### Story 3.1.1: Fix the `notificationTypeFilter` "info" allow-list bug

**As a** user, **I want** an Input-Required notification to never be
treated as merely informational, **so that** it can't be swallowed into a
collapsed section when it's genuinely something needing my attention.

**Acceptance Criteria**:
- `"question"` (Input-Required) is never returned by the `"info"` category.
  - *Given* `types: ["question", "info", "auto_approved"]`, *When*
    `notificationTypeFilter("info", types)` is called, *Then* it returns
    `["info", "auto_approved"]` — not `"question"`.
- A hypothetical future notification type not yet in any explicit case
  falls into `"info"` by default (allow-list behavior), matching today's
  `default: return "info"` in `mapNotificationType`.
  - *Given* a new UI type `"reminder"` not mentioned in any of
    `notificationTypeFilter`'s other three categories, *When*
    `notificationTypeFilter("info", ["reminder"])` is called, *Then* it
    returns `["reminder"]`.
- An unmapped *backend* `NotificationType` proto value never silently lands
  in the informational bucket at the upstream `mapNotificationType` stage
  (pre-mortem.md P1) — the allow-list fix in Task 3.1.1a is worthless if a
  type never reaches it as anything other than `"info"` in the first place.
  - *Given* a numeric proto value with no explicit `case` in
    `mapNotificationType` (a value outside the current `NotificationType`
    enum, standing in for a future backend addition), *When*
    `mapNotificationType(value)` is called, *Then* it logs a warning and
    returns an actionable `UIType` (`isActionableNotification(result) ===
    true`), not `"info"`.

**Files**: `web-app/src/lib/utils/notificationMapping.ts`, `web-app/src/lib/utils/notificationMapping.test.ts`

##### Task 3.1.1a: Convert the "info" case to an allow-list, derived from `ACTIONABLE_TYPES` rather than a second independent list (~5 min)
- **Single source of truth, not a second allow-list (adversarial-review.md
  Concern 3 / this plan's Concern 8)**: this task's original fix converts
  `notificationTypeFilter`'s "info" case from an exclusion-list to an
  allow-list specifically to stop a future notification type from silently
  defaulting into the wrong bucket. Task 3.1.2a then independently defines a
  *second* allow-list, `ACTIONABLE_TYPES` (5 types) — with no shared source
  of truth between the two. Confirmed against `NotificationData`'s
  `notificationType` union (`web-app/src/lib/types/notification.ts:16`,
  13 members total): the originally-drafted informational set (7 types) plus
  `ACTIONABLE_TYPES` (5 types) covers only 12 of the 13 — `"task_complete"`
  falls into neither, which is exactly the silent-drift failure mode this
  story exists to eliminate, just in a new spot. Fix: move `ACTIONABLE_TYPES`
  and `isActionableNotification` (originally Task 3.1.2a) here, and define
  "info" as their literal complement instead of a second enumerated set —
  a type can't be actionable *and* miss both lists once there's only one
  list:
  ```ts
  const ACTIONABLE_TYPES = new Set<UIType>(["approval_needed", "question", "error", "task_failed", "warning"]);
  export function isActionableNotification(type: UIType): boolean {
    return ACTIONABLE_TYPES.has(type);
  }
  ```
  Replace `case "info": return types.filter((t) => t !== ... )`
  (`notificationMapping.ts:118-127`) with:
  ```ts
  case "info":
    return types.filter((t) => !isActionableNotification(t));
  ```
  This makes exhaustiveness structural rather than test-enforced: every
  `UIType` is either actionable (in `ACTIONABLE_TYPES`) or informational (not
  in it) — there is no third, unclassified bucket a future 14th type could
  fall into. Task 3.1.2a (below) now just reuses this export instead of
  redefining it.
- Files: `web-app/src/lib/utils/notificationMapping.ts`

##### Task 3.1.1b: Add regression tests (~4 min)
- Add `test("info category never includes question")` and
  `test("info category includes an unrecognized-but-mapped type")`. No test
  file exists yet for this util (`ls web-app/src/lib/utils/notificationMapping.test.ts`
  confirms absent) — create it, following the naming/setup convention of an
  adjacent util test file (e.g. `notificationGrouping.test.ts` if present).
- Add `test("every UIType is classified by isActionableNotification, none
  fall through both notificationTypeFilter('info') and ACTIONABLE_TYPES")`:
  iterate the full 13-member `notificationType` union
  (`web-app/src/lib/types/notification.ts:16`) and assert each type is
  either in `ACTIONABLE_TYPES` or returned by
  `notificationTypeFilter("info", [type])` — never neither. This is the
  regression test for the specific gap found in review (`"task_complete"`
  fell into neither list under the first draft's two-independent-lists
  design); the complement-based fix in Task 3.1.1a makes this structurally
  guaranteed, but the test pins it against the type union drifting instead
  of just relying on inspection.
- Files: `web-app/src/lib/utils/notificationMapping.test.ts` (new)

##### Task 3.1.1c: Close the upstream `mapNotificationType` default-to-info gap (pre-mortem P1) (~5 min)
- **Root cause**: Task 3.1.1a's allow-list fix operates on `UIType` values
  that `mapNotificationType` (`notificationMapping.ts:17-45`) has *already*
  bucketed from the raw backend `NotificationType` proto number. That
  function's own `default: return "info"` is the actual first place a value
  gets classified — any proto `NotificationType` without an explicit `case`
  here (a future backend addition, or an existing one nobody wired up) is
  already `"info"` by the time `isActionableNotification`/
  `notificationTypeFilter` ever see it, reproducing this project's founding
  bug (an actionable notification silently miscategorized as informational)
  one stage earlier than the bug this story exists to fix.
- **Fix — fail loud, default safe, no new UIType member**: keep every
  existing explicit `case` unchanged; change only the `default` branch so an
  unmapped value (a) is visible for triage instead of disappearing and (b)
  defaults to the actionable side rather than the informational one — the
  same "surfacing is safer than hiding" reasoning pre-mortem.md gives for
  this failure mode:
  ```ts
  default:
    // Fail-safe: an unmapped backend NotificationType must not silently
    // land in the informational bucket — that's the exact bug Task 3.1.1a
    // fixes one stage downstream. Warn for triage and default to an
    // actionable type so it surfaces in NeedsDecisionSection instead of
    // disappearing into collapsed informational activity.
    console.warn(`[notificationMapping] Unmapped NotificationType ${protoType}; defaulting to "warning" (actionable fail-safe)`);
    return "warning";
  ```
  `"warning"` is chosen over `"approval_needed"` specifically because it
  carries no approve/deny action-button assumptions elsewhere in the
  rendering pipeline — it's already in `ACTIONABLE_TYPES` (Task 3.1.1a) and
  renders as a plain, non-interactive card.
- Add a test-time exhaustiveness check that doesn't require a second
  maintained list (avoiding this same story's own lesson about
  independently-drifting allow-lists): iterate every real member of the
  generated `NotificationType` proto enum and assert `console.warn` is never
  called for any of them, then assert calling `mapNotificationType` with a
  value outside the enum *does* warn and returns an actionable type. This
  catches a newly-added backend enum value with no corresponding `case` the
  next time this suite runs against a freshly-regenerated proto, without
  needing the test to enumerate a duplicate list of "expected" cases.
- Files: `web-app/src/lib/utils/notificationMapping.ts`,
  `web-app/src/lib/utils/notificationMapping.test.ts`

#### Story 3.1.2: "Needs a decision" section always on top

**As a** user, **I want** unread actionable notifications pinned above
everything else, **so that** I can tell at a glance whether anything
actually needs me right now.

**Acceptance Criteria**:
- Unread approval/question/error/task_failed/warning notifications render
  in the "Needs a decision" section, uncollapsed, above the fold.
  - *Given* notification history containing one unread `approval_needed`
    item for session `sess-a1b2c3` and three read `task_complete` items,
    *When* `NotificationsPage` renders, *Then* the `approval_needed` item
    appears inside a `NeedsDecisionSection` rendered before the
    informational list, and the three `task_complete` items appear only in
    the collapsed informational section below.
- The empty state is calm and affirmative, not generic.
  - *Given* notification history with zero unread actionable items, *When*
    `NotificationsPage` renders, *Then* `NeedsDecisionSection` shows "All
    caught up" with a checkmark icon — not "No items found" or an empty
    ghost-box illustration.
- The informational section stays visible (collapsed) even when "Needs a
  decision" is empty — it doesn't disappear or expand to fill the space.
- **The bulk "Mark all read" button never marks a `NeedsDecisionSection`
  item as read (Product Triad Review blocker fix — see Task 3.1.2e).**
  - *Given* notification history containing one unread `approval_needed`
    item for `sess-a1b2c3` and three unread `task_complete` items, *When*
    the header's bulk-read button is clicked, *Then* the `approval_needed`
    item remains unread and inside `NeedsDecisionSection`, and only the
    three `task_complete` items (plus any unread Auto-handled items) flip
    to read.
  - *Given* the same state, *Then* the button reads "Mark activity read",
    not "Mark all read", and is enabled only because the `task_complete`
    items are unread — it is disabled/hidden whenever Recent Activity and
    Auto-handled have zero unread items between them, even if
    `NeedsDecisionSection` has unread items of its own.
- **No per-item ✕/dismiss control renders on a `NeedsDecisionSection` item
  (Product Triad Review round-2 blocker fix — see Task 3.1.2b).**
  - *Given* an unread `approval_needed` item rendered inside
    `NeedsDecisionSection`, *When* the item renders, *Then* no element with
    `aria-label="Remove notification"` is present for it.
  - *Given* the same item rendered inside the Recent Activity or
    Auto-handled section instead (once read/resolved), *Then* the ✕ control
    is present and calling it still removes the record from local history
    exactly as it does today — this fix removes the control from one
    section only, it does not remove `removeFromHistory` itself.

**Files**: `web-app/src/app/notifications/NotificationsPage.tsx`, `web-app/src/lib/utils/notificationMapping.ts`, `web-app/src/components/ui/NotificationItem.tsx` (new component), `web-app/src/components/ui/Collapsible.tsx` (reused, not modified), `web-app/src/lib/hooks/useNotificationHistory.ts` (`markAsRead` already accepts an explicit ID list — Task 3.1.2e reuses it instead of `markAllAsRead()`'s empty-list "mark all" semantics; Task 3.1.2h also adds its `lastUpdatedAt` field here), `web-app/src/lib/contexts/NotificationContext.tsx` (Task 3.1.2h exposes `historyError`/`historyLastUpdatedAt`)

##### Task 3.1.2a: Reuse `isActionableNotification` (~1 min)
- No new code — `ACTIONABLE_TYPES`/`isActionableNotification` were moved
  into Task 3.1.1a so `notificationTypeFilter`'s "info" case and this
  story's tiering share one definition instead of two independently
  maintained lists (adversarial-review.md Concern 3). This task is now just
  the import at the call site in Task 3.1.2c.
- Files: `web-app/src/lib/utils/notificationMapping.ts` (already covered by
  Task 3.1.1a)

##### Task 3.1.2b: Add `NeedsDecisionSection` component with calm empty state, without wiring in unscoped `removeFromHistory` (~7 min)
- New exported component in `NotificationItem.tsx` (alongside
  `AutoHandledSection`), taking the same shape of props
  (`notifications`, plus the existing `resolvedApprovals`/`pendingApprovals`/
  `blockedApprovals`/`resolveApproval`/`handleNotificationClick`/
  `getSessionHref` already threaded through `NotificationsPage.tsx`),
  rendering each item via the existing `NotificationItem` (grouped via
  existing `groupNotifications`), with an `aria-live="polite"` region around
  just the count/heading (per ux.md's accessibility guidance — not the full
  list) and the "All caught up" empty state when the filtered list is empty.
  Announcement text is singular for a count of exactly 1 ("1 item needs a
  decision") and plural otherwise ("N items need a decision") — per
  `design/ux.md` AC30.
- **Do not wire `removeFromHistory` into this section (Product Triad Review
  round-2 blocker fix).** `NotificationItem`'s existing ✕ control
  (`NotificationItem.tsx:161-167`) calls `removeFromHistory(notification.id)`
  unconditionally — it deletes the record from local history with zero
  interaction with the underlying approval/session state. Reusing it
  verbatim for `NeedsDecisionSection` items would let a user "dismiss" a
  still-pending `approval_needed`/`question`/`error`/`task_failed`/`warning`
  item while the real decision stays unresolved server-side — the single-item
  version of the exact "items don't stay resolved" failure mode Task 3.1.2e
  fixes for the bulk "Mark all read" button (`design/ux.md` AC7).
  **Fix — make the ✕ control's prop optional and simply don't pass it
  here**, rather than inventing a confirmation dialog or a new "collapse
  detail" affordance this section doesn't otherwise have:
  - Change `NotificationItem`'s `removeFromHistory` prop from required
    (`(id: string) => void`) to optional (`(id: string) => void | undefined`,
    i.e. `removeFromHistory?: ...`), and render the ✕ button only when it's
    provided: `{removeFromHistory && (<button className={removeButton}
    onClick={() => removeFromHistory(notification.id)} aria-label="Remove
    notification">✕</button>)}`.
  - `NeedsDecisionSection` renders its child `NotificationItem`s without a
    `removeFromHistory` prop at all — the ✕ control simply does not render
    for these items. Every other existing caller (Recent Activity, and
    `NotificationPanel`'s compact dropdown) keeps passing
    `removeFromHistory` unchanged, so their ✕ control is unaffected — this
    is additive, not a behavior change for any already-safe (read/resolved/
    informational) item.
  - This also means an item in `NeedsDecisionSection` can only leave the
    section via Approve/Deny/Open session or its underlying state resolving
    elsewhere — never via a ✕ click — matching `design/ux.md` Surface 1's
    corrected interaction flow and AC7.
- **Focus management on removal** (design/ux.md AC 34): when an item that
  was keyboard-focused leaves this section (resolved via Approve/Deny/Open
  session), move focus to the next remaining item, or to the
  section's own heading if it was the last one — standard
  accessible-list-removal handling, not a new pattern this component needs
  to invent (the same handling Task 2.3.2c needs on the Review Queue side).
- **Filter-aware empty state, not a bare `notifications.length === 0` check
  (Product Triad Review round-4 blocker fix).** `notifications` is already
  the *filtered* set (Task 3.1.2c computes it from `filteredNotifications`,
  which has the page's type/search/hide-backlog filters already applied) —
  rendering the calm "All caught up" copy whenever that filtered list is
  empty is wrong whenever a filter, not a lack of actionable items, is what
  emptied it: a user with e.g. the type filter set to "Error" while the only
  unread actionable item is an `approval_needed` would see "All caught up"
  exactly when it's false, contradicting this project's Success Metric the
  same way the already-fixed unscoped "Mark all read" did (`design/ux.md`
  AC6/AC7). Fix — two new props, both supplied by Task 3.1.2c:
  - `totalActionableCount: number` — the `!isRead` + `isActionableNotification`
    count over the *unfiltered* `notificationHistory`.
  - `onClearFilter: () => void` — resets the page's filter state.
  Render rule: `notifications.length === 0 && totalActionableCount === 0` →
  the existing calm "All caught up" empty state (unchanged); `notifications
  .length === 0 && totalActionableCount > 0` → "`{totalActionableCount}`
  item(s) need a decision but are hidden by your filter" with a "Clear
  filter" button calling `onClearFilter` — this branch can only be reached
  when a filter is actually active, since with none applied `notifications`
  and the unfiltered set are identical, so no separate `hasActiveFilter`
  prop is needed on this component. Matches `design/ux.md` AC18 and
  Surface 4's "hidden by filter" wireframe.
- Files: `web-app/src/components/ui/NotificationItem.tsx`

##### Task 3.1.2c: Wire `NotificationsPage.tsx` to render two tiers (~5 min)
- Split `filteredNotifications` into `needsDecision` (unread +
  `isActionableNotification`) and `recentActivity` (everything else) —
  named `recentActivity`, not `informational`, so a future contributor
  grepping for "informational" lands only on Task 3.2.1a's Review Queue tier
  (which is both code-named and displayed as "Informational") rather than
  two same-named-but-different variables on two different pages. Computed
  via `useMemo` alongside the existing `filteredNotifications` (`:85-109`).
- **Compute the true unfiltered actionable count (Task 3.1.2b's
  `totalActionableCount` prop) over `notificationHistory`, never
  `filteredNotifications`**: `notificationHistory.filter(n => !n.isRead &&
  isActionableNotification(n.notificationType)).length` (matching the
  `isRead` field name `useNotificationHistory.ts` already uses), via its own
  `useMemo` alongside the one above — deliberately a separate memo over the
  unfiltered source, not derived from `needsDecision.length`, since the
  whole point is that the two can diverge under an active filter.
- Add a `clearFilters` callback — `() => { setSearchQuery("");
  setTypeFilter("all"); setHideBacklogItems(false); }` — resetting exactly
  the three state variables the existing `hasActiveFilter` (`:111`) already
  tracks, so the new "Clear filter" control can never drift out of sync with
  what the page itself considers "a filter is active." This mirrors
  `ReviewQueuePanel.tsx`'s existing `clearAllFilters` (`:884-908`) in intent
  — `NotificationsPage.tsx` has no equivalent helper today, so this is new
  code here, not a reused function, but it follows the same pattern rather
  than inventing a different reset shape.
- Render `<NeedsDecisionSection notifications={needsDecision}
  totalActionableCount={totalActionableCount} onClearFilter={clearFilters}
  .../>` above the existing `<div className={list}>` block, and restrict
  that existing block's `groupNotifications(...)` call to `recentActivity`
  instead of `filteredNotifications`, wrapped in a `CollapsibleSection`
  (collapsed by default) instead of always-expanded.
- **Reorder the page's existing outer branch so the tier-aware "hidden by
  filter" check runs before the legacy whole-list-empty branch (Product
  Triad Review round-5 blocker fix, `design/ux.md` AC18).** The pre-existing
  ternary at `NotificationsPage.tsx:242-259`
  (`historyLoading && notificationHistory.length === 0 ? <skeleton> :
  filteredNotifications.length === 0 ? <legacy "No matching
  notifications"/"No notifications yet"> : <list>`) sits ahead of, and
  bypasses, `NeedsDecisionSection`'s own filter-aware empty-state logic
  entirely: when a filter empties *both* `needsDecision` and
  `recentActivity` at once (e.g. the type filter set to "Error" while every
  unread notification in history is `task_complete`),
  `filteredNotifications.length === 0` is true, the branch containing
  `NeedsDecisionSection` never executes, and the page falls through to the
  generic "No matching notifications" message with no signal that
  `totalActionableCount` is nonzero — reproducing, via this un-audited
  legacy branch, the exact BLOCKER Task 3.1.2b already fixed for the case
  where `recentActivity` stays non-empty. **Fix — reorder the existing
  branches; do not add a fourth rendering path:**
  1. `historyLoading && notificationHistory.length === 0` → loading
     skeleton (unchanged, highest precedence).
  2. `totalActionableCount > 0 && needsDecision.length === 0` → render
     `<NeedsDecisionSection notifications={[]} totalActionableCount=
     {totalActionableCount} onClearFilter={clearFilters} .../>` (its own
     "hidden by filter" branch, Task 3.1.2b — unchanged) **plus** the
     Recent Activity `CollapsibleSection` if `recentActivity.length > 0`.
     This branch is evaluated, and can fire, regardless of whether
     `recentActivity` is also empty.
  3. `filteredNotifications.length === 0` — now reachable **only** when
     `totalActionableCount === 0`, since step 2 already caught every case
     where it's nonzero — renders the legacy "No matching
     notifications"/"No notifications yet" empty state, copy unchanged.
  4. Otherwise → `<NeedsDecisionSection notifications={needsDecision}
     totalActionableCount={totalActionableCount}
     onClearFilter={clearFilters} .../>` above the Recent Activity
     `CollapsibleSection`, as originally specified.
  Steps 2 and 4 call the identical `NeedsDecisionSection` component with the
  identical prop shape — only the `notifications` array (`[]` vs
  `needsDecision`) and whether the Recent Activity section renders
  alongside it differ. No new component, no new copy variant.
- Files: `web-app/src/app/notifications/NotificationsPage.tsx`

##### Task 3.1.2d: Golden-fixture empty-state test (~4 min)
- Add a test asserting the exact empty-state copy/icon renders when
  `needsDecision` is empty and `recentActivity` is non-empty (both sections
  present, the recent-activity section still visible and collapsed) — per this repo's
  `fix-flaky-tests-dont-defer` convention of testing state transitions
  concretely rather than relying on manual QA.
- **Add a second case (Product Triad Review round-4 blocker fix, `design/ux.md`
  AC18): with an unread `approval_needed` item present but the type filter
  set to "Error", assert the "hidden by filter" copy renders (`"1 item needs
  a decision but is hidden by your filter"`) with a working "Clear filter"
  button — clicking it resets the type filter and the item reappears in
  `NeedsDecisionSection` — and assert the calm "All caught up" copy does
  NOT render in this state.**
- **Add a third case (Product Triad Review round-5 blocker fix, `design/ux.md`
  AC18): with an unread `approval_needed` item present, the type filter set
  to "Error", and no other (non-actionable) notification in history at all —
  i.e. `filteredNotifications.length === 0` AND `recentActivity.length ===
  0` simultaneously, unlike the second case above where `recentActivity`
  stays non-empty — assert the "hidden by filter" copy still renders and the
  legacy "No matching notifications" 🔍 copy does NOT, proving Task 3.1.2c's
  reordered branch takes precedence even when the Recent Activity tier is
  also empty. This is the exact scenario the round-4 fix's own test (the
  second case above) didn't exercise, and where the un-audited legacy
  `filteredNotifications.length === 0` branch used to win.**
- Files: `web-app/src/app/notifications/__tests__/NotificationsPage.test.tsx`

##### Task 3.1.2e: Scope "Mark all read" to Recent Activity + Auto-handled, rename to "Mark activity read" (Product Triad Review blocker fix) (~5 min)
- **Root cause, verified against the actual current code**: the header's
  bulk-read button (`NotificationsPage.tsx:161-170`) calls
  `markAllAsRead()`, which sends `MarkNotificationReadRequest{
  notificationIds: [] }` — server-side shorthand for "mark every
  notification read" (`useNotificationHistory.ts:142-161`). Once
  `NeedsDecisionSection` (Task 3.1.2b/c) exists, this button unconditionally
  empties it in one click, including unread `approval_needed`/`question`/
  `error`/`task_failed`/`warning` items that were never actually resolved —
  the exact "items don't stay resolved" failure mode this whole project
  exists to fix, and one this design's other 27 (now 37, see
  `design/ux.md`) acceptance criteria never covered before this fix.
- **Fix — scope, don't gate behind a confirmation**: replace the
  `markAllAsRead()` call with `markAsRead(ids)`, where `ids` is the list of
  currently-unread notification IDs in `recentActivity` (Task 3.1.2c's
  variable) plus `autoHandledNotifications` (Task 3.1.3a's variable) —
  never `needsDecision`'s IDs. `markAsRead` already accepts an explicit ID
  list (`useNotificationHistory.ts:118-139`); this task adds no new API
  surface, just changes which IDs the button passes.
- Rename the button's visible label and `aria-label` from "Mark all read" /
  "Mark all as read" to "Mark activity read" / "Mark activity as read" —
  the smaller, cleaner fix per `design/ux.md`'s reasoning: once the action
  can no longer touch an unresolved decision, a confirmation dialog has
  nothing left to protect against, but a generic "Mark all read" label
  still invites the reasonable assumption that it means literally
  everything, so the label itself needs to name the actual scope.
- Change the button's render condition from `unreadCount > 0` (which
  includes `needsDecision`'s own unread count) to "Recent Activity or
  Auto-handled has at least one unread item" — otherwise the button can
  render enabled while doing nothing, if every unread item happens to be in
  `NeedsDecisionSection`.
- **Share the scoping logic with `NotificationPanel` (Product Triad Review
  round-3 blocker fix — see Story 3.1.5)**: extract the "which IDs are safe
  to bulk-mark-read" computation into one exported helper,
  `computeScopedMarkReadIds(notifications: NotificationHistoryItem[]):
  string[]` (returns the unread IDs where `!isActionableNotification(type)`),
  in `web-app/src/lib/utils/notificationMapping.ts` next to
  `isActionableNotification`. This task's `ids` variable becomes a call to
  that helper instead of an inline filter, so Task 3.1.5a's dropdown fix
  reuses the identical function rather than a second hand-written filter
  that could drift from this one.
- Files: `web-app/src/app/notifications/NotificationsPage.tsx`,
  `web-app/src/lib/utils/notificationMapping.ts`

##### Task 3.1.2f: Tests for the scoped bulk-read button (~4 min)
- Add a test asserting that clicking the bulk-read button with both an
  unread `approval_needed` item and unread `task_complete` items present
  leaves the `approval_needed` item unread and inside `NeedsDecisionSection`
  afterward, and marks only the `task_complete` items read.
- Add a test asserting the button is absent/disabled when
  `NeedsDecisionSection` has unread items but Recent Activity and
  Auto-handled do not.
- Add a test asserting the button's visible text is "Mark activity read".
- Files: `web-app/src/app/notifications/__tests__/NotificationsPage.test.tsx`

##### Task 3.1.2g: Test that `NeedsDecisionSection` never renders the ✕ control (Product Triad Review round-2 blocker fix) (~3 min)
- Add a test asserting an unread `approval_needed`/`question`/`error` item
  rendered inside `NeedsDecisionSection` has no element with
  `aria-label="Remove notification"`.
- Add a test asserting the same underlying item, once read and rendered in
  Recent Activity (via existing `NotificationItem` usage there), does still
  render that control and clicking it still calls `removeFromHistory` — this
  is a scoping fix, not a removal of the feature.
- Files: `web-app/src/app/notifications/__tests__/NotificationsPage.test.tsx`,
  `web-app/src/components/ui/NotificationItem.test.tsx` (if it exists;
  otherwise the `NotificationsPage` test suite covers both cases)

##### Task 3.1.2h: Background poll/fetch-failure staleness indicator + retry (`design/ux.md` AC38) (~5 min)
- **Root cause, verified against the actual current code**:
  `useNotificationHistory.ts`'s `fetchHistory` already sets an `error` state
  on failure (`:87-93`) and leaves whatever `notifications`/`unreadCount` it
  last successfully fetched untouched — but `NotificationContext.tsx` never
  forwards that `error` (nor any last-successful-fetch timestamp) to
  `NotificationsPage.tsx`. A fetch failure is therefore currently invisible
  to the user: the page just keeps showing whatever it last had, with no
  signal it might be stale — exactly the "silently continuing to show stale
  data as if it were current" failure `design/ux.md`'s Surface 1 edge case
  and AC38 both call out, and one this plan's other 3.1.2 tasks never
  covered.
- **Fix — expose existing state, add one timestamp field, no new polling
  loop**:
  - Add one `useState<number | null>` (`lastUpdatedAt`) to
    `useNotificationHistory.ts`, set to `Date.now()` at the end of
    `fetchHistory`'s success path (`:68-86`, right after `setUnreadCount`/
    `setHasMore`) — never touched in the `catch` branch, so it always holds
    the time of the last *successful* fetch. Add it to
    `UseNotificationHistoryReturn`.
  - Add `historyError: Error | null` and `historyLastUpdatedAt: number |
    null` to `NotificationContextValue`, sourced directly from
    `useNotificationHistory()`'s existing `error` and the new
    `lastUpdatedAt` — passthrough only, no new state duplicated in the
    context.
- In `NotificationsPage.tsx`, render a small text line next to
  `NeedsDecisionSection`'s heading — "Last updated `<Xm ago>` · Retry" —
  only when `historyError` is non-null. Format the elapsed time with a
  minute-rounding inline helper (reuse an existing relative-time utility if
  this codebase already has one; otherwise a one-line `Math.round((Date.now()
  - historyLastUpdatedAt) / 60000)` helper, not a new dependency). "Retry"
  calls `refreshHistory()` (already exposed by `NotificationContext`)
  directly and immediately — no debounce/backoff, matching
  `design/ux.md`'s "re-runs the same poll immediately" note. Once a
  subsequent fetch succeeds, `historyError` clears and the indicator
  disappears on the next render; no separate dismiss action needed.
- **Never let this indicator imply an empty result is fresher than it is**:
  it renders next to `NeedsDecisionSection`'s heading regardless of which of
  that section's three states (normal list / calm empty / hidden-by-filter,
  Task 3.1.2b) is currently showing, including when the last successful
  fetch was itself empty — matching `design/ux.md`'s explicit requirement
  that a stale empty state must never read as current.
- Files: `web-app/src/lib/hooks/useNotificationHistory.ts`,
  `web-app/src/lib/contexts/NotificationContext.tsx`,
  `web-app/src/app/notifications/NotificationsPage.tsx`

##### Task 3.1.2i: Tests for the staleness indicator (~3 min)
- Add a test asserting that when `historyError` is set (simulate a rejected
  `getNotificationHistory` call), the "Last updated `<Xm ago>` · Retry" text
  renders next to `NeedsDecisionSection`'s heading and the last-known list
  content is still shown (not replaced by an error takeover).
- Add a test asserting clicking "Retry" calls `refreshHistory`, and that
  once the mocked RPC subsequently succeeds, the indicator disappears.
- Add a test asserting the indicator does NOT render when there has never
  been a fetch failure (the common case) — guards against a regression that
  makes it always-on.
- Files: `web-app/src/app/notifications/__tests__/NotificationsPage.test.tsx`

#### Story 3.1.3: Auto-handled section extended to reconciled items

**As a** user, **I want** rule-reconciled approvals visible in the same
"Auto-handled" section as live auto-decisions, **so that** I have one place
to audit everything the classifier has ever done on my behalf.

**Acceptance Criteria**:
- A record with `metadata.reconciled === "true"` appears in
  `AutoHandledSection`, not in the main "Needs a decision"/informational
  feed.
  - *Given* a notification `{notificationType: "approval_needed",
    metadata: {approval_decision: "allow", reconciled: "true"}}`, *When*
    `NotificationsPage` computes `autoHandledNotifications`, *Then* this
    record is included; *When* it computes `filteredNotifications`, *Then*
    this record is excluded.

**Files**: `web-app/src/app/notifications/NotificationsPage.tsx`

##### Task 3.1.3a: Extend the auto-handled filter and exclude reconciled items from the main feed (~3 min)
- Use the shared `isReconciledNotification` helper (Task 2.3.2b) at both
  call sites instead of a third independent inline
  `metadata?.reconciled === "true"` comparison.
- Change `notificationHistory.filter((n) => n.notificationType !==
  "auto_approved")` (`:86`) to also exclude `isReconciledNotification(n)`.
- Change `autoHandledNotifications`'s filter (`:120-123`) to
  `n.notificationType === "auto_approved" || isReconciledNotification(n)`.
- Files: `web-app/src/app/notifications/NotificationsPage.tsx`

##### Task 3.1.3b: Test (~2 min)
- Add a test asserting a reconciled `approval_needed` record lands in
  `AutoHandledSection`'s rendered list, not the main list.
- Files: `web-app/src/app/notifications/__tests__/NotificationsPage.test.tsx`

#### Story 3.1.4: Cap the headline unread count like `NavBadge` already does

**As a** user, **I want** the page header to stop showing an ever-growing
raw number, **so that** the count reads as manageable rather than as
evidence the page is broken.

**Acceptance Criteria**:
- The header's unread badge caps at "99+", matching `NavBadge.tsx`'s
  existing convention.
  - *Given* `unreadCount: 152`, *When* the header renders, *Then* it shows
    "99+", not "152".
  - *Given* `unreadCount: 7`, *When* the header renders, *Then* it shows
    "7", unchanged.

**Files**: `web-app/src/app/notifications/NotificationsPage.tsx`

##### Task 3.1.4a: Cap the header's unread count (~2 min)
- At `:152-156`, replace the raw `{unreadCount}` with the same cap
  `NavBadge.tsx:46` uses: `{unreadCount > 99 ? "99+" : unreadCount}`. Extract
  a tiny shared `capBadgeCount(n: number)` helper into
  `web-app/src/lib/utils/notificationMapping.ts` (or a more general shared
  util file if one already exists for this) so `NavBadge.tsx` and this page
  share one definition instead of two copies of the same `> 99` literal.
- Files: `web-app/src/app/notifications/NotificationsPage.tsx`,
  `web-app/src/components/ui/NavBadge.tsx`

##### Task 3.1.4b: Test (~2 min)
- Add a test asserting the header shows "99+" for `unreadCount > 99`.
- Files: `web-app/src/app/notifications/__tests__/NotificationsPage.test.tsx`

---

#### Story 3.1.5: Scope `NotificationPanel`'s bulk-read, ✕, and "Clear all" to the same rule as the Notifications page (Product Triad Review round-3 blocker fix)

**As a** user, **I want** the header bell dropdown to protect a still-pending
decision exactly as reliably as the full Notifications page does, **so
that** I can't accidentally lose track of (or permanently delete) an
unresolved `approval_needed`/`question`/`error` from the one notification
surface that's reachable from every page.

**Root cause, verified against the actual current code**: `NotificationPanel.tsx`
(`web-app/src/components/ui/NotificationPanel.tsx`) is a third live exit path
for exactly the failure mode Tasks 3.1.2b/3.1.2e already fixed once for the
Notifications page — but it was never itself audited as a surface (see
`design/ux.md`'s corrected Surface inventory). Concretely, as of this repair
round:
- Its "Mark all read" button (`NotificationPanel.tsx:147-153`) calls the
  same unscoped `markAllAsRead()` Task 3.1.2e is replacing on the
  Notifications page — untouched by that task because that task's Files list
  never included this component.
- Its `NotificationItem` usage (`NotificationPanel.tsx:220-230`) passes
  `removeFromHistory` unconditionally to every item in its flat list,
  including unread actionable ones — Task 3.1.2b's fix only stops
  `NeedsDecisionSection` from passing the prop; `NotificationPanel` has no
  such section and was never touched.
- Its "Clear all" button (`NotificationPanel.tsx:155-161`) calls `clearHistory()`
  → `ClearNotificationHistoryRequest` (`server/services/notification_service.go:230-261`),
  which deletes every notification record matching an optional
  `before_timestamp` cutoff and nothing else — including a pending
  `approval_needed`/`question`/`error`/`task_failed`/`warning` record, with
  no confirmation dialog. **This exists identically on
  `NotificationsPage.tsx:171-178` too** — Task 3.1.2e never touched "Clear
  all" at all, on either surface, and no other task in this plan does either.

**Acceptance Criteria**: `design/ux.md` AC35-38 (Surface 11).

**Files**: `web-app/src/components/ui/NotificationPanel.tsx`,
`web-app/src/components/ui/NotificationItem.tsx`,
`web-app/src/app/notifications/NotificationsPage.tsx`,
`web-app/src/lib/utils/notificationMapping.ts`,
`server/notifications/store.go`, `proto/session/v1/session.proto`
(doc-comment only — no new field)

##### Task 3.1.5a: Scope `NotificationPanel`'s bulk-read button, reusing Task 3.1.2e's shared helper (~3 min)
- Replace the `onClick={markAllAsRead}` call (`NotificationPanel.tsx:149`)
  with `onClick={() => markAsRead(computeScopedMarkReadIds(notificationHistory))}`,
  using the `computeScopedMarkReadIds` helper Task 3.1.2e now exports from
  `notificationMapping.ts` — no second filter written here.
- Change the button's render condition (currently `unreadCount > 0`,
  `:146`) to "at least one ID would be returned by
  `computeScopedMarkReadIds`" — same reasoning as Task 3.1.2e: never shown
  active with nothing it can affect.
- Rename the label/`aria-label` from "Mark all read"/"Mark all as read" to
  "Mark activity read"/"Mark activity as read", matching Task 3.1.2e's
  wording exactly so the same action reads identically on both surfaces.
- Files: `web-app/src/components/ui/NotificationPanel.tsx`

##### Task 3.1.5b: Exempt unread actionable items from the ✕ control, per item (~3 min)
- At the `<NotificationItem ... removeFromHistory={removeFromHistory} .../>`
  call site (`NotificationPanel.tsx:227`), pass it conditionally instead of
  unconditionally:
  ```tsx
  removeFromHistory={
    isActionableNotification(group.notification.notificationType) && !group.notification.isRead
      ? undefined
      : removeFromHistory
  }
  ```
  This relies on `NotificationItem`'s `removeFromHistory` prop already being
  optional (Task 3.1.2b made it `removeFromHistory?: (id: string) => void`)
  and already only rendering the ✕ button when the prop is provided — no
  further change needed in `NotificationItem.tsx` itself, since this task
  reuses that exact mechanism rather than adding a new one.
- Import `isActionableNotification` from `notificationMapping.ts`.
- Files: `web-app/src/components/ui/NotificationPanel.tsx`

##### Task 3.1.5c: Guarantee `NotificationHistoryStore.Clear()` never deletes an unread actionable record (~6 min)
- **Why this must be a server-side fix, not a client-side filter**:
  `ClearNotificationHistoryRequest` (`proto/session/v1/session.proto:1661-1665`)
  carries only an optional `before_timestamp` cutoff — there is no ID list or
  per-item exclusion parameter for a client to even express "delete
  everything except these." A frontend-only fix could at best refuse to call
  the RPC at all when *any* protected item exists anywhere in the store
  (blocking the whole action, including for old, unrelated resolved
  history), which is worse than a real fix and still wouldn't protect a
  second client (e.g. another browser tab) calling the same RPC directly.
  Fixing it once in `Clear()` protects every current and future caller
  identically, including `NotificationsPage.tsx`'s own "Clear all" (see this
  story's root-cause note above — that button has the same gap and gets the
  same fix, for free, from this one change).
- Add a small Go-side mirror of the frontend's `isActionableNotification`
  (`notificationMapping.ts`), following this project's own established
  pattern of a same-concept Go/TS accessor pair rather than a single
  cross-language source of truth (see the `IsReconciled()`/
  `isReconciledNotification()` pair in the Domain Glossary):
  ```go
  // IsActionableType reports whether t is one of the backend NotificationType
  // values that map to a UI type NeedsDecisionSection/NotificationPanel treat
  // as "needs a decision" (notificationMapping.ts's ACTIONABLE_TYPES: approval_needed,
  // question, error, task_failed, warning). Mirrors that TS set; there is no
  // single cross-language source of truth, so a change to one must be mirrored
  // in the other (same accepted pattern as NotificationRecord.IsReconciled()).
  func IsActionableType(t sessionv1.NotificationType) bool {
      switch t {
      case sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED,
          sessionv1.NotificationType_NOTIFICATION_TYPE_CONFIRMATION_NEEDED,
          sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED,
          sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR,
          sessionv1.NotificationType_NOTIFICATION_TYPE_FAILURE,
          sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING:
          return true
      default:
          return false
      }
  }
  ```
- In `Clear()` (`store.go:326-352`), change both branches (`before == nil`
  and the `kept` loop) so a record is retained whenever
  `!r.IsRead && IsActionableType(r.NotificationType)`, regardless of the
  `before` cutoff or whether `before` was even supplied — this is an
  unconditional invariant of `Clear()`, not a caller-supplied option, mirroring
  `design/ux.md` AC7's "there is no click that silently clears a still-pending
  item" being a hard rule rather than a toggle:
  ```go
  func (s *NotificationHistoryStore) Clear(before *time.Time) (int, error) {
      s.mu.Lock()
      defer s.mu.Unlock()

      originalLen := len(s.records)
      var kept []*NotificationRecord
      for _, r := range s.records {
          if !r.IsRead && IsActionableType(r.NotificationType) {
              kept = append(kept, r) // never delete a still-pending decision
              continue
          }
          if before != nil && r.CreatedAt.Before(*before) {
              continue // aged out
          }
          if before == nil {
              continue // "clear everything" still respects the guard above
          }
          kept = append(kept, r)
      }
      s.records = kept
      // ... unchanged: cleared-count computation, saveToDisk() call.
  }
  ```
- Update `Clear()`'s doc comment to state the guarantee explicitly: "never
  removes a record that is unread and actionable, regardless of the
  before-timestamp cutoff."
- Files: `server/notifications/store.go`

##### Task 3.1.5d: Rename "Clear all" → "Clear history" and add a confirmation dialog, on both surfaces (~4 min)
- On both `NotificationPanel.tsx:155-161` and `NotificationsPage.tsx:171-178`:
  rename the visible label and `aria-label` from "Clear all"/"Clear all
  notifications" to "Clear history"/"Clear notification history", and wrap
  the existing `onClick={clearHistory}` in a native `window.confirm("Clear
  read notifications? This can't be undone. Items still needing a decision
  won't be cleared.")` guard, calling `clearHistory()` only if confirmed —
  same pattern already used by `ReviewQueuePanel.tsx`'s existing "Skip all"
  confirm (`:650-652`), reused rather than inventing a new dialog component.
- No change to `clearHistory()`'s call signature or the RPC request shape —
  the exclusion guarantee lives entirely in Task 3.1.5c's server-side fix;
  this task only changes the button's copy and adds the confirm gate.
- Files: `web-app/src/components/ui/NotificationPanel.tsx`,
  `web-app/src/app/notifications/NotificationsPage.tsx`

##### Task 3.1.5e: Tests (~6 min)
- `store_test.go`: add `TestClear_NeverDeletesUnreadActionableRecord_WithTimestampCutoff`
  and `TestClear_NeverDeletesUnreadActionableRecord_ClearEverything` (the
  `before == nil` path) — seed an unread `APPROVAL_NEEDED` record alongside
  an old, read `TASK_COMPLETE` record, call `Clear(nil)` and `Clear(&future)`
  respectively, and assert the approval-needed record survives both while
  the task-complete record is removed.
- `NotificationPanel.test.tsx` (create if it doesn't exist yet — check
  first): add a test asserting the bulk-read button never marks an unread
  `approval_needed` item read, a test asserting no ✕ control renders for
  that same item, and a test asserting the ✕ control does render (and still
  works) for a read/informational item in the same list.
- Add a shared test case (parameterized over both `NotificationPanel` and
  `NotificationsPage`, or duplicated if no shared test harness exists for
  both) asserting "Clear history" is a no-op on an unread `approval_needed`
  item and requires confirmation before it removes anything else.
- Files: `server/notifications/store_test.go`,
  `web-app/src/components/ui/NotificationPanel.test.tsx` (new, or existing
  location — check first),
  `web-app/src/app/notifications/__tests__/NotificationsPage.test.tsx`

---

### Epic 3.2: Review Queue Page IA

#### Story 3.2.1: Priority-tier sectioning

**As a** user, **I want** the review queue to visually separate items that
need a decision from low-priority noise, **so that** I stop scanning past
47 idle rows to find the two that actually matter.

**Acceptance Criteria**:
- Items at `PriorityUrgent`/`PriorityHigh`/`PriorityMedium` render in an
  always-expanded "Needs a decision" section; `PriorityLow` items render in
  a collapsed-by-default "Informational" section.
  - *Given* review-queue items `[{sessionId: "sess-a", priority:
    PriorityHigh, reason: "approval_pending"}, {sessionId: "sess-b",
    priority: PriorityLow, reason: "stale"}]`, *When* `ReviewQueuePanel`
    renders with no grouping strategy selected, *Then* `sess-a` renders in
    the expanded top section and `sess-b` renders in the collapsed bottom
    section.
- The existing `groupingStrategy` (Category/Tag/Branch/etc.) still applies
  *within* each tier when selected, not instead of the tiering.
- The empty "needs a decision" state uses the same calm, affirmative
  treatment as Story 3.1.2's Notifications-page empty state (per ux.md:
  "this is the page's actual success condition").

**Verified, no task needed — "Skip all (N)" does not need Mark-all-read's
scoping fix (Product Triad Review follow-up check)**: the concern raised
against the old unscoped "Mark all read" — a bulk action silently and
durably clearing an item that still needs a decision — was checked against
`handleSkipAllVisible` (`ReviewQueuePanel.tsx:647-680`) and found not to
apply. `skippableItems` already excludes every item carrying
`metadata["pending_approval_id"]` (approval-pending items must be
Approve/Deny'd, never bulk-dismissed), and the action is already gated by a
native `window.confirm()` before it runs (`:650-652`). For the other
higher-priority reasons the action *can* still reach
(`input_required`/`error_state`/`tests_failing`), `Determine()`
(`session/review_queue_determiner.go:106-260`, confirmed by reading it) —
unlike Idle/Stale — applies no acknowledgment-suppression to any of those
three: a skip on one of them does not persist past the next ~2s poll tick
if the underlying condition still holds. So "Skip all" cannot silently and
durably clear a genuinely pending decision the way the old "Mark all read"
could; no code change or confirmation beyond the existing dialog is added
here. See `design/ux.md` Surface 6's interaction flow (point 4) for the
user-facing writeup of this same finding.

**Files**: `web-app/src/components/sessions/ReviewQueuePanel.tsx`, `web-app/src/lib/store/reviewQueueSlice.ts` and `web-app/src/lib/hooks/useReviewQueue.ts` (Task 3.2.1d's `lastUpdatedAt` field)

##### Task 3.2.1a: Partition items into two priority tiers ahead of existing grouping (~5 min)
- Add a `useMemo` splitting `items` (post-filter, pre-`groupingStrategy`)
  into `needsDecisionItems` (`priority !== PriorityLow`) and
  `informationalItems` (`priority === PriorityLow`), each independently fed
  through the existing `groupSessions(sessions, groupingStrategy)` call
  (`:615`) when a grouping strategy is active.
- **Also compute `allNeedsDecisionCount` — `priority !== PriorityLow` over
  `allItems` (`:378`), the pool *before* any of the
  priority-include/exclude/reason/severity/program/category/tag/PR/diverged/
  search filters in `allFilteredItems` (`:446-538`) apply — not over `items`
  (Product Triad Review round-4 blocker fix, `design/ux.md` AC18).** This is
  what Task 3.2.1c's empty state compares against `needsDecisionItems.length`
  to tell "genuinely zero urgent/high/medium items" apart from "some exist
  but the active filter is hiding all of them."
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 3.2.1b: Render the two tiers via `CollapsibleGroup`/`CollapsibleSection` (~5 min)
- Wrap the existing per-item rendering loop into two sections: the "Needs a
  decision" tier always expanded, the "Informational" tier wrapped in a
  `CollapsibleSection` collapsed by default (reusing
  `web-app/src/components/ui/Collapsible.tsx`, not building a new
  accordion).
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 3.2.1c: Calm empty state + golden fixture test (~4 min)
- Add the same "All caught up" treatment as Task 3.1.2b's empty state when
  `needsDecisionItems` is empty **and** Task 3.2.1a's `allNeedsDecisionCount`
  is also `0`.
- **Filter-aware empty state, not a bare `needsDecisionItems.length === 0`
  check (Product Triad Review round-4 blocker fix, `design/ux.md` AC18).**
  The panel's existing whole-list `hasActiveFilter` empty state
  (`:1559-1573`, "No items match the current filter") only fires when
  `items.length === 0` overall — it does not cover the case where
  `informationalItems` is still non-empty but the needs-decision tier alone
  has been filtered down to zero (e.g. `reasonFilter = {IDLE_TIMEOUT}` while
  the only remaining urgent item's reason is `APPROVAL_PENDING`). Render
  rule, using Task 3.2.1a's `allNeedsDecisionCount`: `needsDecisionItems
  .length === 0 && allNeedsDecisionCount === 0` → calm "All caught up"
  (unchanged); `needsDecisionItems.length === 0 && allNeedsDecisionCount >
  0` → "`{allNeedsDecisionCount}` item(s) need a decision but are hidden by
  your filter" with a "Clear filter" button. **Reuse the panel's existing
  `clearAllFilters` (`:884-908`) for that button — the identical function
  and button the whole-list empty state already calls at `:1566-1572` — not
  a second, tier-scoped implementation.**
- **Reorder the panel's existing outer branch so this check runs before the
  legacy whole-list-empty branch (Product Triad Review round-5 blocker fix,
  `design/ux.md` AC18).** The pre-existing branch at
  `ReviewQueuePanel.tsx:1556-1589` (`loading && items.length === 0 ?
  <loading> : items.length === 0 ? <hasActiveFilter ? "No items match the
  current filter" : hadItems ? "All done!" : "No sessions need attention!">
  : <two-tier list>`) sits ahead of, and bypasses, the tier-aware check
  above entirely: when a filter empties *both* `needsDecisionItems` and
  `informationalItems` at once (e.g. `reasonFilter = {IDLE_TIMEOUT}` while
  every remaining item in the whole queue, informational tier included, has
  reason `IDLE_TIMEOUT`), `items.length === 0` is true, and the panel falls
  through to the legacy "No items match the current filter" message with no
  signal that `allNeedsDecisionCount` is nonzero — reproducing, via this
  un-audited legacy branch, the exact BLOCKER Task 3.2.1a/c already fixed
  for the case where `informationalItems` stays non-empty. **Fix — reorder
  the existing branches; do not add a fourth rendering path:**
  1. `loading && items.length === 0` → loading state (unchanged, highest
     precedence).
  2. `allNeedsDecisionCount > 0 && needsDecisionItems.length === 0` → render
     the "hidden by filter" needs-decision tier (this bullet's rule above,
     unchanged) **plus** the Informational `CollapsibleSection` if
     `informationalItems.length > 0`. Evaluated, and can fire, regardless of
     whether `informationalItems` is also empty.
  3. `items.length === 0` — now reachable **only** when
     `allNeedsDecisionCount === 0`, since step 2 already caught every case
     where it's nonzero — renders the legacy `hasActiveFilter`/`hadItems`
     empty states, copy unchanged.
  4. Otherwise → the normal two-tier render (Task 3.2.1b).
  Steps 2 and 4 render the identical needs-decision tier markup — only
  whether `needsDecisionItems` is empty (triggering its own "hidden by
  filter" copy) and whether the Informational section renders alongside it
  differ. No new component, no new copy variant.
- Add a golden-fixture test asserting the two-tier split renders correctly
  for a mixed-priority fixture, and that an empty needs-decision tier shows
  the calm state while the informational tier stays visible-but-collapsed.
- **Add a second fixture case: a mixed-priority queue with `reasonFilter`
  set to exclude the only urgent/high/medium item's reason, asserting the
  "hidden by filter" copy renders with the correct `allNeedsDecisionCount`
  and that clicking "Clear filter" (the same `clearAllFilters` button)
  brings the item back into `needsDecisionItems` — and asserting the calm
  "All caught up" copy does NOT render in this state.**
- **Add a third fixture case (Product Triad Review round-5 blocker fix,
  `design/ux.md` AC18): a mixed-priority queue where `reasonFilter` excludes
  every item's reason — both the urgent/high/medium item's and the
  low-priority item's — so `items.length === 0` overall (unlike the second
  case above where `informationalItems` stays non-empty). Assert the
  "hidden by filter" copy still renders with the correct
  `allNeedsDecisionCount`, and the legacy "No items match the current
  filter" copy does NOT, proving the reordered branch takes precedence even
  when the Informational tier is also empty — the exact scenario the
  round-4 fix's own test (the second case above) didn't exercise.**
- Files: `web-app/src/components/sessions/__tests__/ReviewQueuePanel*.test.tsx`
  (existing test file(s) in this directory — extend rather than create a
  new file if one already covers `ReviewQueuePanel` rendering).

##### Task 3.2.1d: Background poll/fetch-failure staleness indicator + retry, reusing Task 3.1.2h's pattern (`design/ux.md` AC38) (~4 min)
- **Root cause, verified against the actual current code**:
  `ReviewQueuePanel.tsx:1196-1205` already has an `error` state (from
  `useReviewQueueContext()`, backed by `reviewQueueSlice.ts`'s `setError`),
  but treats any fetch error as a full-panel takeover —
  `if (error) return <div>Failed to load review queue...</div>` — replacing
  the entire queue, including any items already successfully loaded.
  Confirmed by reading `reviewQueueSlice.ts:53-55`: `setError` only writes
  `state.error`, it never clears `state.reviewQueue`, so `allItems` is still
  the last-known-good data at the moment this branch discards it. This is
  the same failure class AC38 exists to prevent, just manifesting as an
  *overly aggressive* takeover instead of a *silent* stale display: a single
  flaky poll blanks a queue the user was already looking at, discarding
  perfectly good stale data instead of just flagging it as possibly stale.
- **Add `lastUpdatedAt` tracking first, mirroring Task 3.1.2h's field on the
  Notifications page — not a differently-shaped mechanism**: add one
  `lastUpdatedAt: number | null` field to `reviewQueueSlice.ts`'s state, set
  to `Date.now()` inside `setReviewQueue` (only reached on a successful
  fetch — `useReviewQueue.ts:152-175` already distinguishes the success path
  from the `catch` branch that calls `setError`), and a matching
  `selectReviewQueueLastUpdatedAt` selector.
- **Fix**: change the full-takeover branch's condition from `if (error)` to
  `if (error && lastUpdatedAt === null)` — **not** `allItems.length === 0`
  (round-6 UX re-check caught this: `allItems.length === 0` conflates "never
  successfully fetched" with "successfully fetched, legitimately empty, then
  a later background poll failed" — both leave `allItems.length === 0`, so a
  transient poll blip on a genuinely-caught-up queue would wrongly trigger
  the full takeover instead of AC38's intended calm-empty-state +
  staleness-indicator). `lastUpdatedAt === null` means exactly "never
  completed a successful fetch," independent of item count, which is the
  correct condition for a first-load failure. When `error` is set but
  `lastUpdatedAt !== null`, render the queue normally (Task 3.2.1a/b/c's
  two-tier split, unaffected — including the calm empty state when
  legitimately caught up) with the same "Last updated `<Xm ago>` · Retry"
  indicator from Task 3.1.2h next to the "Needs a decision" tier heading,
  wired to the existing `refresh` function (`useReviewQueueContext().refresh`,
  already in scope at `:386`) — no new RPC or retry mechanism.
- Files: `web-app/src/lib/store/reviewQueueSlice.ts`,
  `web-app/src/lib/hooks/useReviewQueue.ts`,
  `web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 3.2.1e: Tests for the staleness indicator and the narrowed error takeover (~3 min)
- Add a test asserting that when `error` is set but `lastUpdatedAt` is
  non-null, the panel still renders the two-tier list (not the full "Failed
  to load" replacement) with the "Last updated `<Xm ago>` · Retry" text
  visible next to the "Needs a decision" heading.
- Add a test asserting the full "Failed to load review queue" takeover still
  renders when `error` is set AND `lastUpdatedAt` is null (first-load
  failure) — guards against accidentally removing that path entirely.
- Add a test asserting the specific case round 6 caught: `error` set,
  `lastUpdatedAt` non-null, AND `allItems` legitimately empty (a genuinely
  caught-up queue whose next background poll failed) — asserts the calm
  empty state renders with the staleness indicator, never the full takeover.
- Add a test asserting clicking "Retry" in the staleness indicator calls
  `refresh`, and that the indicator disappears once a subsequent fetch
  succeeds.
- Files: `web-app/src/components/sessions/__tests__/ReviewQueuePanel*.test.tsx`

#### Story 3.2.2: Idle items no longer occupy Review Queue slots; Sessions list gets the chip back

**As a** user, **I want** an idle "ready for next task" session to show up
as a small status chip on the Sessions list instead of as a review-queue
row, **so that** the queue only ever contains things that actually need a
decision.

**Accepted limitation, verified (pre-mortem.md #3)**: combining Epic 1.2's
ack-suppression with this story's unconditional removal of idle items raises
the concern that a session skipped once while genuinely idle, then later
truly stuck (tmux pane dies, a call hangs, no `StatusError`/
`StatusTestsFailing` transition ever fires), would be suppressed forever
with only the passive Sessions-list chip as a signal. Checked against the
actual code rather than assumed: `StaleSessionNotifier`
(`server/services/stale_session_notifier.go:26-35`) is a genuinely separate,
independently-tuned sweeper that "does not read, modify, or share dedup
state with any of" the review-queue's own staleness detectors, and fires an
edge-triggered, self-clearing notification the moment an **`Active`**
session's `GetTimeSinceLastMeaningfulOutput()` crosses
`config.StaleSession.ThresholdMinutesOrDefault()` — independent of the
review queue's `Reason`/ack-suppression state entirely, so a session
suppressed out of the queue by Epic 1.2 is not solely dependent on the
passive chip: it still gets its own notification once truly stuck (as long
as it remains `Active`, which the pre-mortem's "hangs with no error
transition" scenario is). Accepted as the compensating control; no code
change proposed here.

**Acceptance Criteria**:
- An idle-reason item never renders in `ReviewQueuePanel`'s list, and the
  review-queue nav badge count excludes it too (no count-vs-list mismatch).
  - *Given* review-queue items `[{sessionId: "sess-c", priority:
    PriorityLow, reason: "idle"}, {sessionId: "sess-d", priority:
    PriorityHigh, reason: "approval_pending"}]`, *When*
    `ReviewQueuePanel` renders and `ReviewQueueNavBadge` computes its count,
    *Then* only `sess-d` appears in the panel and the nav badge shows `1`,
    not `2`.
- **The panel's own headline `totalItems` count also excludes idle items —
  not just the `items` array (adversarial-review.md Blocker 2).** The
  original task filtered only the `items` array; `TotalItems` is a
  *separate* stat (`session/queue/queue.go:490`,
  `TotalItems: len(rq.items)`, over *all* items including idle ones, since
  ADR-002 deliberately leaves the backend `Determine()`/queue unchanged) that
  flows through `WatchReviewQueue` into `reviewQueueSlice.ts`'s
  `stats.totalItems`/`reviewQueue.totalItems`
  (`web-app/src/lib/store/reviewQueueSlice.ts:8,28,44,63-64,73-74`) and is
  rendered verbatim by `ReviewQueuePanel.tsx` at lines ~1222, ~1250-1253,
  and ~1564 (verify exact line numbers against current code before editing —
  this plan was written against a specific revision) — e.g. `"{totalItems}
  items in queue"`, literally the style of inflated number
  (Problem Statement: "107 review-queue items") this project exists to fix.
  Left unfixed, this reproduces the exact bug in a number ADR-002's own
  reasoning claims is solved "by construction."
  - *Given* the same two items as above, *When* `ReviewQueuePanel` renders
    its headline count, *Then* it reads "1 item in queue", not "2 items in
    queue".

**Files**: `web-app/src/lib/utils/reviewQueueVisibility.ts` (new),
`web-app/src/lib/hooks/useReviewQueue.ts`, `web-app/src/components/sessions/SessionRow.tsx`,
`web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 3.2.2a: Add `isReviewQueueVisible`, filter `items` once centrally, and republish a filtered `totalItems` (~6 min)
- New file `web-app/src/lib/utils/reviewQueueVisibility.ts`:
  ```ts
  import type { ReviewItem } from "@/lib/types/reviewQueue"; // match existing import path used elsewhere for this type
  export function isReviewQueueVisible(item: Pick<ReviewItem, "reason">): boolean {
    return item.reason !== "idle";
  }
  ```
- In `useReviewQueue.ts`, apply `isReviewQueueVisible` once to the array
  used as `items` (currently `liveItems` from
  `selectReviewQueueItemsWithLiveStatus`, confirmed at
  `useReviewQueue.ts:119,502`), so every consumer (`ReviewQueuePanel`,
  `ReviewQueueNavBadge`, `BottomNav`, `DrawerNav`) gets a filtered view by
  construction — avoiding the count-vs-list mismatch class of bug
  `StuckItemsSection`'s own comment warns about (features.md §1).
- **`totalItems` fix (adversarial-review.md Blocker 2)**: the raw backend
  stat (`reviewQueue?.totalItems`, currently returned verbatim at
  `useReviewQueue.ts:478`) counts *all* items including idle ones — it is
  not derived from `items` at all, so filtering `items` alone doesn't fix
  it. Rather than touch `session/queue/queue.go` or
  `WatchReviewQueue`'s wire protocol (out of scope per requirements.md's
  Out-of-Scope list), republish `totalItems` in this hook as the *filtered*
  `items.length` instead of passing the raw backend stat through:
  ```ts
  const filteredItems = liveItems.filter(isReviewQueueVisible);
  // ...
  return {
    items: filteredItems,
    totalItems: filteredItems.length, // was: reviewQueue?.totalItems ?? 0
    // ...
  };
  ```
  This is the smaller, more consistent diff than threading a new
  `NeedsDecisionCount` field through `queue.go` → the RPC/streaming layer →
  `reviewQueueSlice.ts` — it keeps `TotalItems`'s backend meaning ("all
  in-memory review items") unchanged for any other consumer of
  `session/queue/queue.go`'s stats while making the one thing users see
  (`ReviewQueuePanel.tsx`'s headline count, sourced from this hook) correct.
  Every existing `ReviewQueuePanel.tsx` read of `totalItems` (lines ~1222,
  ~1250-1253, ~1564 — verify against current code) needs no further change
  once this hook's return value is fixed, since they all read `totalItems`
  from this same hook rather than the raw Redux stat directly.
- Files: `web-app/src/lib/utils/reviewQueueVisibility.ts`, `web-app/src/lib/hooks/useReviewQueue.ts`

##### Task 3.2.2b: Stop suppressing the IDLE chip in `SessionRow.tsx` (~2 min)
- Remove `session.subStatus !== SubStatus.IDLE &&` from the condition at
  `SessionRow.tsx:352`. Update `SubStatusChip.tsx`'s doc comment (`:26-27`)
  — "SessionRow filters out IDLE and READY" becomes "...filters out READY"
  only.
- Files: `web-app/src/components/sessions/SessionRow.tsx`, `web-app/src/components/sessions/SubStatusChip.tsx`

##### Task 3.2.2c: Tests (~5 min)
- Update `SessionRow.test.tsx` if it currently asserts IDLE is suppressed
  (flip the assertion). Add a test to `ReviewQueuePanel`'s test file
  asserting an idle item never renders and the nav-badge test asserting its
  count excludes idle items.
- Add `test("useReviewQueue totalItems excludes idle items")` (or an
  equivalent `ReviewQueuePanel` render-level test asserting the headline
  count) covering adversarial-review.md Blocker 2 — given a mix of idle and
  non-idle items, assert `totalItems` equals the filtered `items.length`,
  not the raw backend stat.
- Files: `web-app/src/components/sessions/SessionRow.test.tsx`,
  `web-app/src/components/sessions/__tests__/ReviewQueuePanel*.test.tsx`,
  `web-app/src/lib/hooks/useReviewQueue.test.ts` (or equivalent existing
  test file for this hook), nav-badge test file if one exists for
  `ReviewQueueNavBadge`.

---

## Regression Safety Checklist (run before shipping, per Constraints)

- `go test ./session/... ./server/...` — must not regress
  `TestReviewQueue*`/`TestReviewQueuePoller*` (explicit Constraint).
- `cd web-app && npx jest --no-coverage` — must not regress
  `NotificationsPage.test.tsx`, `ReviewQueue.focus.test.tsx`,
  `notification-policy.test.ts`, `notifications.test.ts`.
- `make quick-check` (build + test + lint) before opening the PR; `make
  ready` (dupl/jscpd gates) before pushing, per this repo's CLAUDE.md.
- E2E: `tests/e2e/` specs touching the Notifications/Review Queue pages
  should be re-run (`cd tests/e2e && npx playwright test`) given the IA
  restructuring in Phase 3 — check for any `data-testid` selectors this
  plan's DOM restructuring might invalidate (e.g. a test asserting a flat
  list order that now expects two sections).
