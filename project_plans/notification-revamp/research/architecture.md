# Architecture Research: notification-revamp

Builds on (do not re-derive):
- `project_plans/review-queue-state-detection/research/03-architecture.md` — proto/event
  options for a `WorkingState` signal; not pursued here since scope item 3 caps this project
  at reusing what's already wired, not adding new proto surface (see §3).
- `project_plans/review-queue-event-driven/research/architecture.md` — `ClaudeController`
  `OnOutput`/`IdleDetector` event wiring; out of scope here per requirements.md (latency
  concern), referenced only where it touches the idle-suppression fix (§2).
- `project_plans/review-queue-severity/research/architecture.md` — traced `RiskLevel`
  end-to-end from classifier to `ReviewItem.metadata["risk_level"]` and `SeverityBadge`.
  **That plan has since shipped** (requirements.md confirms "merged #411") — §5 below verifies
  the current frontend state against that plan's file list and finds it fully implemented.

## 1. Notification dedup fix — `server/notifications/store.go`

### Minimal-diff change

`Append()` (`server/notifications/store.go:138-176`) already does everything the fix needs
*except* one condition. The dedup lookup is `findUnreadDuplicate` (`store.go:212-219`):

```go
func (s *NotificationHistoryStore) findUnreadDuplicate(sessionID string, notifType int32) *NotificationRecord {
	for _, r := range s.records {
		if r.SessionID == sessionID && r.NotificationType == notifType && !r.IsRead {
			return r
		}
	}
	return nil
}
```

The minimal-diff fix is to **drop the `!r.IsRead` condition** — rename to `findDuplicate` (or
keep the name and just remove the read-state filter) so it matches on `(sessionID,
notificationType)` alone, then in the collapse branch (`store.go:150-165`) explicitly flip
`IsRead` back to `false` when collapsing into a previously-read record, since a new occurrence
of an already-seen event type is new information the user hasn't seen yet:

```go
if existing := s.findDuplicate(record.SessionID, record.NotificationType); existing != nil {
	if record.NotificationType == notifTypeApprovalNeeded {
		existing.ID = record.ID
	}
	existing.OccurrenceCount++
	existing.LastOccurredAt = &record.CreatedAt
	existing.Message = record.Message
	existing.Metadata = record.Metadata
	existing.Title = record.Title
	existing.IsRead = false   // NEW: a repeat occurrence un-reads the record
	existing.ReadAt = nil     // NEW: clear stale read timestamp
	s.moveToFront(existing)
	return s.saveToDisk()
}
```

This is a ~4-line diff: remove `!r.IsRead` from the lookup predicate, add two field resets in
the collapse branch. No new fields, no schema change — `OccurrenceCount`/`LastOccurredAt`
already exist and already do the right thing once the record is found.

### What breaks if done naively — the ADR-003 read-state fork exists on purpose

The in-file comment block at `store.go:130-133` documents the *current* contract precisely:
"if the existing record is already read, a new unread record is created instead" — this is
"ADR-003" and it is not an oversight, it is the thing requirements.md's Failure Mode 1 says is
wrong. Two concrete downstream assumptions depend on read-state transitions that the naive
"always collapse" fix must not break:

1. **`notifTypeApprovalNeeded`'s ID-reassignment side effect** (`store.go:153-155`,
   `notifTypeApprovalNeeded = 1`). When collapsing, `existing.ID` is overwritten with the
   *incoming* approval's UUID so that `SetMetadata("newApprovalID", ...)` (outcome-stamping,
   looked up by record ID after the user acts on the approval) still finds the record. This
   already runs unconditionally inside the found-duplicate branch — removing the `!r.IsRead`
   guard does not disturb it, since the ID reassignment doesn't care whether the record was
   previously read. Confirmed no separate code path relies on an approval-type record staying
   at its *original* ID across a read/unread cycle.
2. **`AppendAutoApproved`'s pre-read invariant** (`store.go:181-207`) sets `IsRead: true` at
   construction and is documented to "never appear in the active notification feed." If
   `findUnreadDuplicate` becomes `findDuplicate` (drops the read filter) *and* the collapse
   branch unconditionally sets `IsRead = false`, a second identical auto-approval (e.g. the
   same rule firing twice) would flip a previously-silent `auto_approved` record back to
   unread and it would incorrectly surface in the main feed instead of staying in the
   "Auto-handled" collapsed section. **This is the one real regression risk of the naive
   fix.** Mitigation: only reset `IsRead = false` when `record.NotificationType !=
   notifTypeAutoApproved` — i.e. keep `AppendAutoApproved`'s pre-read contract intact by
   special-casing exactly the one type that already has a distinct contract (it already
   bypasses the event bus per its own doc comment), rather than special-casing every type
   individually.
3. **`deduplicateExisting()`** (called once at load time, `store.go:122-124`, not shown above
   but referenced by `NewNotificationHistoryStore`) is a startup migration that "consolidates
   unread duplicates into single records" — its docstring already says *unread* duplicates.
   Confirm (Phase 3/5) whether it also needs the same predicate change so that records
   persisted under the old model (multiple rows, one read + N unread, for the same
   `(sessionID, type)`) get consolidated on next load too — otherwise the old forked rows
   linger until they age out via `MaxNotificationAge` (7 days, `store.go:19`) or are read
   individually. Not fixing this is not incorrect, just leaves pre-existing forked rows
   un-consolidated; low priority given the 500-row/7-day retention cap already bounds the
   damage (`store.go:16-19`).

No other file reads `NotificationRecord.IsRead` to key off a read→unread transition
specifically (as opposed to just "is this currently unread" for counting) — confirmed by
grep; the only structural risk is the `AppendAutoApproved` interaction in point 2.

## 2. Idle ack-suppression — extend the Stale pattern to Idle, no special-casing

### The Stale pattern (`session/review_queue_determiner.go:278-307`)

```go
timeSinceOutput := inst.GetTimeSinceLastMeaningfulOutput()
alreadyAcknowledged := inst.IsAcknowledgedAfterOutput()

if timeSinceOutput > d.config.StalenessThreshold {
	if alreadyAcknowledged {
		// skip — flagged as stale but suppressed
	} else if !shouldAdd || priority.IsLowerThan(PriorityMedium) {
		reason = ReasonStale
		...
	}
}
```

`IsAcknowledgedAfterOutput()` (`session/review_state.go:172-190`) is **not** reason-scoped —
it's a pure function of two session-wide, lock-free timestamps: `LastAcknowledged` (set by
`Instance.MarkAcknowledged()`, `session/instance_state.go:246-257`, the sole caller being
`server/services/review_queue_service.go:194` — i.e. whatever RPC handles "skip/acknowledge"
on *any* review-queue item) vs. `LastMeaningfulOutput` (updated by `ReviewState.UpdateTimestamps`
whenever terminal content signature changes, `session/review_state.go:217-242`). It answers
"has the user dismissed this session more recently than anything new happened," independent
of *why* the session was flagged. This is exactly the semantics the idle bug needs — "skipping
an idle item doesn't stick" — because the same acknowledgment call already exists and already
updates the same timestamp for every reason type; only the Idle branches never check it.

### Two idle sites, both need the identical one-line guard

There are two places `ReasonIdle` is set, and neither currently reads `IsAcknowledgedAfterOutput()`:

1. **Controller-active path**, `IdleStateTimeout` case (`review_queue_determiner.go:178-184`):
   ```go
   case detection.IdleStateTimeout:
   	reason = ReasonIdle
   	priority = PriorityLow
   	shouldAdd = true
   	ctx = "Session idle - ready for next task"
   ```
2. **No-controller path**, time-based fallback (`review_queue_determiner.go:257-265`):
   ```go
   if !shouldAdd {
   	const basicIdleThreshold = 5 * time.Second
   	if time.Since(inst.UpdatedAt) > basicIdleThreshold {
   		reason = ReasonIdle
   		priority = PriorityLow
   		shouldAdd = true
   		ctx = "Session idle - ready for next task"
   	}
   }
   ```

**Extension, "no special-casing" reading**: rather than duplicating the Stale block's
if/else shape twice more (once per site), hoist the check into a tiny shared predicate reused
by both — e.g. a private helper on `DefaultStatusDeterminer`:

```go
// suppressedByAck reports whether the given reason is currently snoozed because the user
// acknowledged the session more recently than its last meaningful output. Applies uniformly
// to any reason set inside Determine — Stale already implements this pattern (see below);
// this generalizes it so a third reason added later gets the same suppression for free.
func (d *DefaultStatusDeterminer) suppressedByAck(inst *Instance) bool {
	return inst.IsAcknowledgedAfterOutput()
}
```

Then guard both `ReasonIdle` assignment sites with `if !d.suppressedByAck(inst) { ... }`
around the existing four-line block, and refactor the Stale block (§L283-307) to call the
same helper instead of its own inline `alreadyAcknowledged := inst.IsAcknowledgedAfterOutput()`
local. This is "the same pattern, extended," not a parallel bespoke mechanism — the fix is
almost entirely deletion-shaped (one shared call site) rather than three independent
near-duplicate acknowledgment checks, which is what "without special-casing" rules out.

**Why this is safe / matches the acceptance criterion**: because `IsAcknowledgedAfterOutput()`
compares against `LastMeaningfulOutput` (not `UpdatedAt`, which the no-controller idle branch
uses for its *threshold* check but not its *suppression* check), acknowledging an idle session
and then having it produce genuinely new output will naturally un-suppress it — new output
advances `LastMeaningfulOutput` past `LastAcknowledged`, exactly the "reappears only when the
underlying state materially changes" requirement in requirements.md's Success Metrics.

**One asymmetry to flag for planning**: the no-controller idle threshold is 5s
(`basicIdleThreshold`, line 259) while the controller-active `IdleStateTimeout` threshold is
whatever `IdleDetectorConfig.IdleThreshold` is configured to (also 5s by default per
`review-queue-state-detection`'s research, `IdleThreshold: 5s`). Both are far shorter than
`StalenessThreshold` (2m, same source). This means an idle acknowledgment can be "used up" by
a very brief lull and need re-acknowledging moments later if content churns even slightly
(each churn updates `LastMeaningfulOutput`, invalidating the ack) — worth a UX gut-check in
planning, but it is the same tradeoff Stale already accepts, just at a shorter timescale.

## 3. Working-state detection reuse — already integrated, not a parallel state machine

**Finding: there is no separate "detection package built for unattended-PTY-write gating"
that needs new integration work.** The `detection` package (`session/detection/`) is a single,
shared vocabulary already fully consumed by the review queue determiner:

- `detection.DetectedStatus` (`detector.go:90` for `StatusIdle`, plus `StatusActive`,
  `StatusExecuting`, `StatusProcessing`, `StatusCompacting`, `StatusWaitingForAgent`, etc.) —
  this is exactly `statusInfo.ClaudeStatus` already switched on at
  `review_queue_determiner.go:136-164` (controller-active) and `:208-253` (no-controller).
- `detection.IdleState` (`idle.go:14-22`: `IdleStateActive`/`IdleStateWaiting`/
  `IdleStateTimeout`) — already the exact enum switched on at
  `review_queue_determiner.go:169-184`.

The "unattended PTY write gating" feature referenced in project memory is
`server/services/session_service.go:981-1015` (`isSafeSteerStatus`/`IsReadyForSteer`, used by
PR-fix auto-steering). It does **not** define its own state machine — it consumes the *same*
`detection.DetectedStatus`/`statusContext` pair the determiner already reads, just narrowed by
an additional allowlist of exact `statusContext` description strings
(`safeIdleStatusContexts`, `session_service.go:990-994`) to distinguish "Claude's own idle
prompt" from "a raw shell/vim prompt that also reports `StatusIdle`." That allowlist is a
steer-safety concern (is it safe to write bytes unattended) with no bearing on review-queue
membership — it is not a reusable review-queue signal itself, but its existence proves the
underlying `DetectedStatus`/`statusContext` vocabulary is already shared safely across two
different consumers with different purposes. **Open Question 1 in requirements.md is answered:
reuse is not just viable, it already happened** — the review queue was built on this state
machine from the start. There is no new plumbing to add for the base case.

### The actual gap: two asymmetric fallback paths, not a missing state machine

Where the mid-turn misclassification risk (review-queue-state-detection's original FR-1..3
concern) still has teeth is in two structural asymmetries within `Determine()` itself, both of
which are fixable without adding new detection infrastructure:

1. **Early-return protection exists for `IdleStateActive` and
   `StatusExecuting/Processing/Compacting`, but only via `return DetectionResult{Action:
   DetectionActionRemove, ...}`** (`review_queue_determiner.go:172` and `:239-240`) — both
   *skip the later Stale check entirely* by returning before reaching L278. This already
   solves the "actively producing output" half of scope item 3 for the common cases; no change
   needed there.
2. **`StatusWaitingForAgent` (the auto-mode background-task footer state) is only handled in
   the no-controller branch** (`review_queue_determiner.go:241-253`, with the
   `waitingForAgentStuckThreshold` 30-minute grace period), **not in the controller-active
   branch's switch** (`:136-164` has no `StatusWaitingForAgent` case at all). For a
   controller-active session showing this status, control falls through to the `idleState`
   switch (`:169-184`) instead, which has no special handling for "background task running,
   footer counter incrementing but bytes may not hash-differ every poll tick" — if
   `IdleDetector` independently reports `IdleStateTimeout` while the auto-mode footer is still
   live, the session could still misclassify as Idle despite being demonstrably mid-task. This
   is a concrete, narrow gap: **add a `case detection.StatusWaitingForAgent:` arm to the
   controller-active switch, mirroring lines 241-253's grace-period logic**, rather than a new
   detection primitive. This satisfies scope item 3's "reusing existing signals... rather than
   building new golden-corpus tooling" instruction precisely — it's copying an existing
   4-line grace-period pattern to a second branch, not new heuristics.

## 4. Rule-reconciliation wiring

### `classifier.Classify()` is a pure, thread-safe function — safe to re-run standalone

`func (c *RuleBasedClassifier) Classify(payload PermissionRequestPayload, ctx
ClassificationContext) ClassificationResult` (`pkg/classifier/classifier.go:456-460`) takes an
`RLock`, delegates to `classifyInternal`, and has no side effects of its own (no notification
writes, no analytics, no queue mutation — those all happen in the caller,
`approval_handler.go`). This means re-running it against a stored `PendingApproval` requires
only reconstructing its two inputs, both of which `PendingApproval` already retains verbatim
(`server/services/approval_store.go:21-53`): `ToolName`, `ToolInput`, `Cwd`, `PermissionMode`
map directly onto `PermissionRequestPayload` fields (`classifier.go:57-65`); `Cwd` also seeds
`ClassificationContext.Cwd`. The one `ClassificationContext` field the stored approval does
*not* retain is `SessionIdleMinutes` (`approval_handler.go:370`, live-computed from
`inst.GetTimeSinceLastMeaningfulOutput()` at request time) — for reconciliation this should be
recomputed live from the current instance state (if the session still exists) rather than
reconstructed from history, since it's meant to reflect "how idle is the session *right now*,"
which is exactly what a reconciliation pass wants to re-evaluate against.

### `ApprovalStore.Resolve()` already handles both live and orphaned approvals uniformly

`func (s *ApprovalStore) Resolve(id string, decision ApprovalDecision) error`
(`server/services/approval_store.go:180-205`) is the existing single entry point for
"terminate a pending approval with a decision" — it removes the record, persists, and either
sends on `decisionCh` (live HTTP handler still blocked waiting) or is a no-op send-skip for
`Orphaned` approvals (server-restart case). **This is precisely the mechanism a reconciliation
pass needs** — it doesn't need a new "resolve without a live request" path; `Resolve()` already
degrades gracefully when there's no live HTTP connection, which is the common case for a
long-pending, previously-escalated approval being reconciled well after creation.

### Trigger points: three call sites, one shared choke point

All rule-change entry points funnel through exactly two `RulesService` rebuild methods, both
already serialized by `rebuildMu` (`server/services/rules_service.go:39-42`):

| Trigger | Call site | Rebuild method |
|---|---|---|
| `UpsertApprovalRule` RPC | `rules_service.go:166` | `rebuildClassifier()` (`:525-534`) |
| `DeleteApprovalRule` RPC | `rules_service.go:186` | `rebuildClassifier()` (`:525-534`) |
| Manual `ReloadClaudeSettingsRules` RPC | `rules_service.go:205` → `claudeSettingsWatcher.Reload()` | `rebuildClaudeSettingsRules()` (`:539-547`) |
| fsnotify auto-reload (`ClaudeSettingsWatcher`, PR #538) | (not read in this pass, but documented as calling the same `onReload` path) | `rebuildClaudeSettingsRules()` |

**Cleanest hook point**: add reconciliation as a call made from *inside* both
`rebuildClassifier()` and `rebuildClaudeSettingsRules()`, immediately after
`rs.classifier.ReplaceRules(...)` — while `rebuildMu` is still held. This is safe (no
deadlock) because `ReplaceRules` and the proposed reconciliation's calls to `Classify()` use
the classifier's *own* internal mutex (`classifier.mu`), a different lock than `rebuildMu`;
holding `rebuildMu` while separately taking/releasing `classifier.mu` via `Classify()` calls
is the same nesting pattern `rebuildClassifier()` itself already does (`Rules()` at
`rules_service.go:530` takes `classifier.mu.RLock` while `rebuildMu` is held). Concretely:

```go
func (rs *RulesService) rebuildClassifier() {
	rs.rebuildMu.Lock()
	defer rs.rebuildMu.Unlock()
	userRules := rs.rulesStore.ToRules()
	existing := rs.classifier.Rules()
	rs.afterRebuildReadHook()
	nonUser := filterRulesBySource(existing, classifier.SourceSeed, classifier.SourceClaudeSettings)
	rs.classifier.ReplaceRules(append(nonUser, userRules...))
	rs.reconcilePendingApprovals() // NEW
}
```

with the identical one-line addition in `rebuildClaudeSettingsRules()`. Both call sites
already exist, already run under the same serialization guarantee, and already represent "a
rule set just changed" — no new trigger, no new event type, satisfying requirements.md's Open
Question 2 ("piggyback on the existing reload path" — confirmed as the better option; a new
explicit `upsert_approval_rule`-only trigger would miss the `ReloadClaudeSettingsRules`/
fsnotify paths, which are just as capable of newly-covering a pending item).

### Wiring `ApprovalStore` into `RulesService`

`RulesService` currently holds no reference to `ApprovalStore` — confirmed by grep, zero hits.
Wiring is trivial because of construction order in `server/services/session_service.go`:
`approvalStore := NewApprovalStore(...)` (line 661) runs **before**
`rulesSvc := NewRulesService(rulesStore, nil, analyticsStore, classifierObj, promptBuilder,
aiClientImpl)` (line 721) in the same function — `approvalStore` is already in scope and can
be passed as a new constructor parameter, or (matching the existing `SetClaudeSettingsWatcher`
setter-injection idiom used for the watcher, itself constructed later than `RulesService`) via
a new `rs.SetApprovalStore(approvalStore)` call. Prefer the setter form for consistency with
the file's own documented idiom ("same setter-injection idiom as SetHistoryLinker/
SetHeadlessPool," `rules_service.go:58-60`) even though a constructor param would also work
given the ordering.

### `reconcilePendingApprovals()` sketch — avoiding live-request side effects

The Feasibility Risk in requirements.md ("run the classifier against historical pending items
without re-triggering side effects meant for live requests") is addressed by construction: the
side effects that matter (auto-approval-log writes, analytics recording, headless-pool LLM
consultation, HTTP response writing) all live in `approval_handler.go`'s `ServeHTTP`-adjacent
code, not in `Classify()` itself. A reconciliation pass calls `Classify()` directly and handles
only the `AutoAllow`/`AutoDeny` outcomes itself (an `Escalate` result means "still needs a
human," so it's a no-op — leave the pending approval as-is):

```go
func (rs *RulesService) reconcilePendingApprovals() {
	if rs.approvalStore == nil {
		return
	}
	for _, a := range rs.approvalStore.ListAll() { // existing method, used by Path A per review-queue-severity research §3
		payload := classifier.PermissionRequestPayload{
			ToolName: a.ToolName, ToolInput: a.ToolInput,
			Cwd: a.Cwd, PermissionMode: a.PermissionMode,
		}
		ctx := classifier.ClassificationContext{Cwd: a.Cwd /* + live IsGitRepo/Env/SessionIdleMinutes as available */}
		result := rs.classifier.Classify(payload, ctx)
		if result.Decision == classifier.Escalate {
			continue // still needs a human — no change
		}
		decision := "deny"
		if result.Decision == classifier.AutoAllow {
			decision = "allow"
		}
		if err := rs.approvalStore.Resolve(a.ID, ApprovalDecision{Behavior: decision, Message: result.Reason}); err != nil {
			continue // already resolved concurrently by the user — fine, skip
		}
		// Visible audit trail per Observability Requirements — see notification-dedup note below.
		rs.notifyAutoResolved(a, result)
	}
}
```

This never touches `approval_handler.go`'s HTTP path, never calls `h.autoApprovalLog` (a
*different* record shape than what the audit note requires — see below), and never invokes
the headless/autonomous LLM approval path (`approval_handler.go:435+`), which is exactly the
"no interference with a request the user is mid-way through answering" requirement — a
reconciled item was already fully escalated and parked; nothing about it resembles an in-flight
live request.

### Visible audit note — reuse `AppendAutoApproved`'s notification type, add a distinguishing field

Requirements.md demands a visible "Auto-resolved by rule: `<name>`" note (never silent, per
`feedback_document_ai_decisions_in_edge_cases`). `AppendAutoApproved`
(`server/notifications/store.go:181-207`) already writes a pre-read `NOTIFICATION_TYPE_AUTO_APPROVED`
(`notifTypeAutoApproved = 13`) record with `ruleID/ruleName/ruleSource` metadata, and the
frontend's "Auto-handled" section (`web-app/src/components/ui/NotificationPanel.tsx:78-104`)
already filters purely on `notificationType === "auto_approved"` — reusing this type for
reconciliation-time resolutions means **zero frontend changes** are needed to satisfy scope
item 5 ("extend the existing Auto-handled section to include rule-reconciled items"). The
distinguishing signal for "this was resolved after being escalated, not decided at request
time" should be a new metadata key (e.g. `"reconciled": "true"` or `"resolved_at":
"<timestamp>"`) rather than a new notification type, since the existing filter/section
plumbing already routes by type alone and a new type would require a second frontend touchpoint
for no product-visible benefit.

## 5. Priority vs. RiskLevel in `ReviewQueuePanel.tsx` — both already fully wired

**The `review-queue-severity` project's plan is not just planned — it has shipped in full,**
confirmed against the current file (all of the following are read directly, not inferred):

| Capability | Priority (`ReviewItem.priority`, session-state enum) | RiskLevel (`metadata["risk_level"]`, classifier enum) |
|---|---|---|
| Badge component | `ReviewQueueBadge` (`ReviewQueuePanel.tsx:983-994`, rendered compact + full) | `SeverityBadge` (`ReviewQueuePanel.tsx:1005`, only when `metadata["pending_approval_id"]` is set) |
| Sort field | `"priority"` in `SORT_FIELDS` (`:155`), comparator at `:521-522` | `"severity"` in `SORT_FIELDS` (`:155`), comparator via `riskLevelRank()` at `:520` |
| Filter (include/exclude) | `priorityFilter`/`priorityExcludeFilter` (`:339-340`, applied `:457-461`) with per-value counts (`byPriority`, `:1337-1347`) | `severityFilter`/`severityExcludeFilter` (`:343`, applied `:469-473`) with `severityFilterKey()` bucketing unrecorded values (`:180-181`) |
| Grouping | `groupingStrategy` (`GroupingStrategy` enum reused from the Sessions list — Category/Tag/Branch/Path/Program/Status/SessionType/None, `:361,613-624`) — **not** a priority/severity-tier grouping | none |

**What is NOT built** (the actual gap for scope item 6, "Review Queue page IA reboot"): there
is no default visual **tiering** — no "Needs a decision" section pinned above a collapsed
"informational" section. The existing `groupingStrategy` groups by session *attributes*
(branch, tag, program, etc.), orthogonal to urgency. Today the queue is a single flat list,
sortable/filterable by priority or severity but not *sectioned* by them, and idle-class items
(hardcoded `PriorityLow`, confirmed still true at `review_queue_determiner.go:160/182/235/262`)
sit in the same list as urgent approvals unless the user manually applies a priority-exclude
filter. **The plan phase's task here is UI/UX only** — a grouping-by-priority-tier view (or a
"pinned above the fold" section for `PRIORITY_URGENT`/`PRIORITY_HIGH` vs. everything else) — no
new backend data, no new proto field, no new sort/filter plumbing. This directly resolves
requirements.md's Open Question 3 groundwork: since `Priority`/`RiskLevel` filtering already
exists and idle items are already the lowest priority tier, "remove idle entirely vs. keep as
lowest visually-collapsed tier" is purely a frontend layout decision now, not a data-availability
question.

## Event-Command-Policy table

Per the domain-complexity check: this project touches four actors (rule editor, classifier,
review-queue determiner, notification store) with one genuinely cross-cutting policy
(auto-resolve-on-rule-change reaching back into already-escalated state). The table below
covers the two flows requirements.md and this research treat as architecturally non-trivial —
notification dedup is single-actor/single-command and is *not* included (a table would
manufacture ceremony for a 4-line diff, same judgment the `review-queue-severity` research
made for its own scope).

| Domain Event | Policy Trigger | Command | Actor / System |
|---|---|---|---|
| `RuleUpserted` (user edits/adds a rule via RPC) | none (direct) | `UpsertApprovalRule` | Human (via UI) → `RulesService` |
| `RuleDeleted` | none (direct) | `DeleteApprovalRule` | Human (via UI) → `RulesService` |
| `ClaudeSettingsFileChanged` (fsnotify) or manual reload | none (direct) | `ReloadClaudeSettingsRules` | fsnotify watcher / Human → `ClaudeSettingsWatcher` → `RulesService` |
| `ClassifierRulesReplaced` (any of the above three completed) | **"whenever rules change, re-check pending escalations"** | `ReconcilePendingApprovals` | `RulesService` (internal, inside `rebuildMu` critical section) |
| `PendingApprovalReclassified` (`Classify()` returned `AutoAllow`/`AutoDeny` for a previously-`Escalate`d item) | **"an auto-resolution must be visible, never silent"** | `Resolve(approvalID, decision)` + `AppendAutoApproved(..., reconciled=true)` | `RulesService` → `ApprovalStore` + `NotificationHistoryStore` |
| `PendingApprovalReclassified` (continued) | **"resolving an approval that backs a review-queue item must clear that item too"** | (existing) `ReviewQueuePoller` reconcile-loop drop of the `ReasonApprovalPending` entry once `GetApprovalMetadataBySession` no longer returns it | `ReviewQueuePoller` (already-existing polling behavior — confirmed no new command needed; the poller already re-derives approval-backed items from `ApprovalStore` each cycle, so removing the record is sufficient) |
| `PendingApprovalReclassified` (still `Escalate`) | none — explicit no-op | (none) | `RulesService` (loop continues) |
| — | — | — | — |
| `NotificationTypeRecurred` (same `sessionID`+`notificationType` fires again, any read state) | **"a recurrence is new information regardless of prior read state"** | `Append(record)` → collapse-and-unread | Any producer (session lifecycle, classifier auto-decision, stale-session notifier, etc.) → `NotificationHistoryStore` |
| `NotificationTypeRecurred` (type == `AUTO_APPROVED`) | **"pre-read auto-handled records must not resurface in the main feed"** | `Append(record)` → collapse, `IsRead` stays `true` | Same producers → `NotificationHistoryStore` (carve-out from the policy above) |

## Summary of files touched per in-scope item (for planning's task breakdown)

| Item | File | Change |
|---|---|---|
| 1. Dedup | `server/notifications/store.go` | `findUnreadDuplicate` → `findDuplicate` (drop read filter); collapse branch resets `IsRead`/`ReadAt` except for `notifTypeAutoApproved`; consider `deduplicateExisting()` parity |
| 2. Idle ack | `session/review_queue_determiner.go` | Extract `suppressedByAck` helper; guard both `ReasonIdle` sites (`:178-184`, `:257-265`); refactor Stale block to use the same helper |
| 3. Working-state | `session/review_queue_determiner.go` | Add `case detection.StatusWaitingForAgent:` to the controller-active switch (`:136-164`), mirroring the no-controller grace-period logic at `:241-253` |
| 4. Reconciliation | `server/services/rules_service.go` | New `approvalStore` field + `SetApprovalStore` setter; new `reconcilePendingApprovals()` called from both `rebuildClassifier()` and `rebuildClaudeSettingsRules()` |
| 4. Reconciliation | `server/services/session_service.go` | Wire `rulesSvc.SetApprovalStore(approvalStore)` after both are constructed (line ~721+) |
| 4. Reconciliation | `server/notifications/store.go` | `AppendAutoApproved` (or a thin variant) gains a `reconciled`/`resolved_at` metadata key |
| 5/6. Review Queue IA | `web-app/src/components/sessions/ReviewQueuePanel.tsx` | New priority-tier sectioning (not new data plumbing — `priority`/`metadata["risk_level"]` already flow end-to-end) |
| 6. Idle chip | `web-app/src/components/sessions/*` (Sessions list) + `session/review_queue_determiner.go` | Decide (Phase 3 UX) whether idle-only items are removed from `Determine()`'s output entirely vs. kept lowest-tier; either way needs a Sessions-list status chip surfacing the same idle signal already computed here |
