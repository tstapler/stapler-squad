# Pitfalls & Risks: notification-revamp

Research Agent 4. Covers the six in-scope changes from `requirements.md`. All
line numbers verified against the current worktree
(`stapler-squad-notification-revamp_18d23f187c6ea783`) unless marked
otherwise.

## 1. Notification dedup fix (`server/notifications/store.go` `Append()`)

**Concurrency**: not a new risk. `Append()` already takes `s.mu.Lock()` for
the whole read-modify-write (existence check → `findUnreadDuplicate` →
mutate-or-insert → `enforceRetention` → `saveToDisk`), all under one
critical section (store.go:139-166). `findUnreadDuplicate` is documented
"must be called with the write lock held" and is. As long as the fix widens
`findUnreadDuplicate`'s matching (drop the `!r.IsRead` filter) without
splitting the critical section, this stays safe. Do not restructure into
separate read-then-write calls — that would reintroduce the exact
lost-update shape `dynamic-rule-reload`'s ADR-002 had to fix for
`RulesService.rebuildClassifier()` (see §4 below).

**Retention prunes by `CreatedAt`, not `LastOccurredAt` — this is the
highest-severity finding in this section.** `enforceRetention()`
(store.go:511-522) prunes any record where `r.CreatedAt` is older than
`MaxNotificationAge` (7 days), and `Append()`'s collapse path never updates
`existing.CreatedAt` — only `LastOccurredAt` (store.go: `existing.LastOccurredAt
= &record.CreatedAt`, no corresponding `existing.CreatedAt = ...`). Today
this is harmless because a repeat-after-read forks a fresh row with a fresh
`CreatedAt`. Once the fix makes `Append()` always collapse regardless of read
state, a long-lived, frequently-recurring low-value notification (exactly
the "Claude turn complete" case named in the Problem Statement) will keep
its original `CreatedAt` forever. The moment that first-occurrence timestamp
crosses 7 days old, `enforceRetention()` deletes the entire row — current
`OccurrenceCount` and unread state included — even though `LastOccurredAt`
may be seconds old. The next occurrence then silently starts a fresh
`OccurrenceCount = 1` row, which is precisely the "unbounded fresh rows"
symptom the fix is meant to eliminate, just on a ~7-day period instead of
per-read. **Fix must change the retention cutoff check to key off
`LastOccurredAt` (falling back to `CreatedAt` when nil, for old records) —
not stop at fixing the read-state fork alone.**

**Frontend assumption check** (`web-app/src/lib/utils/notificationGrouping.ts`):
`groupNotifications()` computes `count = max(representative.occurrenceCount,
group.length)` and groups by `(sessionId, notificationType)`. This does NOT
assume records fork on read — it already treats `occurrenceCount` as
authoritative and `group.length` only as a stale-data fallback. No change
needed here; the fix makes the `group.length` fallback path a no-op (there
will only ever be one record per key after the fix, once
`deduplicateExisting()` cleans up pre-fix duplicates on next load — already
wired at store.go's `NewNotificationHistoryStore`). Verified: no double-count
risk.

However, `NotificationContext.tsx`'s hydration reconciliation
(`useEffect` at line 146) builds `backendByDedupKey` as a plain `Map` from
`backendItems` in array order — if stale pre-fix duplicate rows for the same
dedup key are still present in a single fetch (possible mid-migration, before
`deduplicateExisting()` has run against on-disk data written by an
old binary), `Map` construction keeps whichever duplicate appears *last* in
the array, not necessarily the most recently updated one. Low risk (only
matters for the one load cycle right after upgrading a running instance) but
worth a note in the plan rather than assuming `deduplicateExisting()` alone
fully covers the transition.

**Occurrence count is a native `int`** — no realistic overflow risk for a
single-operator tool. The existing "zero-value from old JSON treated as 1"
comment (store.go:47-48) already covers the one edge case worth knowing
about; `deduplicateExisting()` applies the same `totalCount == 0 → 1`
correction on load (store.go:472-475), so no new handling needed here.

## 2. Idle ack-suppression

**What went wrong in `review-gate-stale-session-rework` (PR #219 /
ADR-001)**: a single shared threshold
(`ReviewQueuePollerConfig.StalenessThreshold`, then 2 minutes) was reused for
two purposes with very different risk profiles — a low-stakes informational
"Stale" badge, and `notifyIfActiveWorkSessionStale`'s gate on whether
`AutoReopenAfterFailedReview` treats a live session as safe to leave alone.
2 minutes was too short for both: normal slow LLM turns routinely exceed it,
so 37 of 41 review-queue items were false-positive "Stale," **and** the
rework-block gate fired on sessions that weren't actually stuck, blocking an
automated action. The fix split one undocumented, unvalidated shared
constant into three named, independently-tuned thresholds
(`docs`-worth: `project_plans/review-gate-stale-session-rework/decisions/ADR-001-staleness-threshold-recalibration.md`).

**Generalization to idle-suppression**: the review queue already has an
ack-suppression primitive — `ReviewState.IsAcknowledgedAfterOutput()`
(`session/review_state.go:172-190`), used today only for `ReasonStale`
(`review_queue_determiner.go:278-307`). It compares `LastAcknowledged` vs
`LastMeaningfulOutput` (both lock-free atomic shadows) and is bypassed for
high-priority "prompt" reasons (`isActiveHighPriority` in
`review_queue_poller.go:822-826`) specifically because the content-signature
dedup in `UpdateTimestamps()` does not advance `LastMeaningfulOutput` when an
identical prompt reappears — without the bypass, a live blocking approval
prompt could get silently snoozed forever.

Extending the same mechanism to `ReasonIdle` is structurally sound (Idle is
already `PriorityLow`, so it's naturally outside the `isActiveHighPriority`
bypass and won't accidentally suppress a real prompt). The risk that *does*
generalize from PR #219 is **threshold mismatch across purposes**: the idle
trigger itself is `basicIdleThreshold = 5 * time.Second`
(`review_queue_determiner.go:259`) for the no-controller path — two to three
orders of magnitude shorter than any of ADR-001's recalibrated values (5min
badge / 15min rework-gate / 2h stuck-work). Applying acknowledgment-based
suppression on top of a 5-second trigger risks: (a) thrashing — a session
that idles for 6 seconds between quick tool calls gets ack-suppressed, then
immediately un-suppressed the moment any new terminal content advances
`LastMeaningfulOutput`, defeating the "stays out of the queue until it
materially changes" success metric; (b) the same class of "mask a genuinely
stuck session" risk PR #219 fixed for Stale — if detection weaknesses (see
§3, P-2 "esc to interrupt" false positive) cause a session that's actually
waiting on input to get classified as `ReasonIdle` instead of
`ReasonInputRequired`/`ReasonWaitingForUser`, ack-suppressing it could hide a
genuinely-blocked session behind a low-priority reason that never resurfaces
until content changes — and a session waiting on the user by definition
won't produce new content. **The fix + check this repo's Constraints section
already demands ("not just a threshold tweak") should include a test/fixture
asserting that a session parked at `ReasonInputRequired`/approval is never
reachable through the idle-suppression path**, mirroring the existing
`isActiveHighPriority` bypass reasoning.

## 3. Working-state detection — reusing the `detection` package

**Don't re-derive `project_plans/review-queue-state-detection/research/04-pitfalls.md`** — its catalog (P-1 through P-7) already covers: pattern
fragility against Claude Code UI changes (P-1), the "esc to interrupt" /
cost-summary false-positive risks from unanchored regex (P-2, P-3), and
`LastMeaningfulOutput`'s fragile update path when no one is viewing the
terminal (P-7, directly relevant since §2's ack-suppression also keys off
`LastMeaningfulOutput`).

**That document is now partially stale — verify before citing it as current
state.** It was written against an earlier snapshot; at least one of its
findings has since been fixed by intervening shipped work (`osc-status-signals`,
`context-compaction-detection`, `session-status-unification`):
P-4 ("scrollback masking — `statusDetectionTailBytes` defined but not wired")
is **no longer true** — `session/claude_controller.go` now calls
`GetRecentHash`/`GetRecentOutputInto` with `statusDetectionTailBytes`
throughout (lines 732, 743, 1007-1013, 1128, 1155), confirmed by direct read.
Treat every item in that doc as "as of its write date," re-verify the ones
that matter to the plan rather than importing the list wholesale — the same
caution applies in reverse: those three intervening plans added new overlay
logic (OSC priority overrides, the "auto-mode footer idle override" mentioned
inline at `review_queue_determiner.go:242-252`) that the pitfalls doc
predates and doesn't cover.

**New finding — confirmed dead state, matches
[[instinct_detection_status_ready_dead_code]]**: `PatternSet.MatchLines`
(`session/detection/pattern_set.go:171-174`) explicitly never returns
`StatusReady` — its catch-all branch is commented "Ready (catch-all — must be
last; returns StatusUnknown so the `.*` pattern renders no badge)" and
literally returns `StatusUnknown`. Yet `StatusReady` is still referenced as a
live, reachable state in at least 9 other call sites across the codebase:
`session/detection/idle.go:234`, `session/autonomous_driver.go:624`,
`session/claude_controller.go:787,1084`, `session/instance_status.go:113,157`,
`session/detection/osc_priority.go:11,26`, `session/detection/proto_mapping.go:28,62`,
`server/adapters/{review_queue_adapter.go:28,instance_adapter.go:322}`,
`session/status_mapping.go:32`. None of these are wrong per se (checking for
an unreachable value in an OR-list is harmless), but it's evidence the
`detection` package carries **dead branches accumulated across at least
three stalled/partial "unification" plans**
(`project_plans/session-status-unification`, `project_plans/osc-status-signals`,
`project_plans/context-compaction-detection` — the latter two shipped, the
first appears to have landed the `pattern_set.go` rename but left the
downstream `StatusReady` references unmigrated). **Risk for this project**:
building working-state detection logic on top of this state machine without
first mapping which of its ~10 `DetectedStatus` values are actually
reachable from `MatchLines` risks quietly depending on a state that can never
fire (silent dead code, not a crash) — e.g. treating `StatusReady` as a
distinct "genuinely idle" signal separate from `StatusUnknown` would be a
no-op. Before wiring any new working-state logic to specific `DetectedStatus`
values, grep `pattern_set.go`'s `MatchLines` return sites directly (as done
here) rather than trusting call-site usage elsewhere in the codebase as proof
a state is live.

## 4. Rule-reconciliation auto-resolve (highest risk item)

**Concurrency: this is explicitly a "third rebuild path" the codebase
already anticipated and warned about.** `dynamic-rule-reload`'s
ADR-002 (`project_plans/dynamic-rule-reload/decisions/ADR-002-rulesservice-mutex-for-reload-serialization.md`)
added `RulesService.rebuildMu` specifically to serialize
`rebuildClassifier()` (triggered by `UpsertApprovalRule`/`DeleteApprovalRule`/
`BulkUpsertRules`) against `rebuildClaudeSettingsRules()` (fsnotify + the
`ReloadClaudeSettingsRules` RPC) — both do an unsynchronized
read-filter-replace against `RuleBasedClassifier.Rules()`/`ReplaceRules()`
that loses updates if not serialized. Its own "Consequences" section warns:
*"a third or fourth rebuild path added later ... must remember to also
acquire `rebuildMu` — this is a discipline requirement, not a
compiler-enforced one."* Rule-reconciliation triggering off the same
upsert/reload path is exactly that anticipated third consumer. It must
either run **after** the triggering `ReplaceRules()` call completes (reading
the now-current classifier state) while still serialized under `rebuildMu`
against any concurrent reload, or explicitly document why it's safe not to.
Skipping this reintroduces the same lost-update class of bug the ADR exists
to prevent, just with reconciliation reading a stale rule set instead of a
reload clobbering another reload.

**Blocking/latency risk from running inside the upsert RPC path.**
`approval_handler.go`'s live classification (`ClassificationContext`) does
real work per item — `classCtx.CIStatus` (fail-closed if stale, gated on
`ghInfo.LastPRStatusCheck`), `classCtx.SessionIdleMinutes` (live instance
lookup), potentially GitHub PR/CI status. If reconciliation re-runs
`Classify()` against every pending `Escalate` item synchronously inside
`UpsertApprovalRule`'s handler while holding `rebuildMu`, a rule edit with
many pending items outstanding would make that RPC call (and the reload path
for every other trigger) block for the full duration of N classify +
live-lookup calls. Recommend reconciliation runs asynchronously after the
rule swap, not inline in the RPC critical section.

**Stale command/classification context.** A pending `Escalate` item was
created with a `ClassificationContext` snapshot from whenever the tool call
first arrived. Re-running `Classify()` now necessarily uses a **freshly
gathered** context (current CI status, current session idle minutes, current
git state) rather than the original one — meaning a pending item can flip to
auto-decide for a reason unrelated to the rule that was just added or edited
(e.g. CI went green in the interim, or the session went idle past a
threshold). The visible "Auto-resolved by rule: `<name>`" audit note
(Observability Requirements) would then be misleading — attributing the
resolution to the rule when surrounding context also changed. The audit
note should ideally capture what context was used at resolution time, or at
minimum the plan should acknowledge this attribution isn't perfectly
precise rather than presenting it as more certain than it is.

Separately: the underlying **tool-call payload itself** (e.g. a file path,
command text) reflects the state of the world at escalation time. If the
file was since modified/deleted, or the git branch changed, re-classifying
the *original* payload and auto-approving it doesn't verify the action is
still safe to apply *now* — it only verifies the classifier's rules would
now permit that historical request. Scope this explicitly: reconciliation
auto-*resolves the pending decision* (matches Success Metrics' "would now
auto-decide"), it must not be read as re-validating that executing the
original action is still safe against current repo state — those are
different guarantees and the UI/audit note should not conflate them.

**Race: human clicks Approve/Deny while reconciliation is running.**
Verified in `server/services/approval_store.go:180-205`: `Resolve()` deletes
the pending entry from `s.pending` under `s.mu.Lock()` *before* sending on
`decisionCh`, and returns an error (`"not found or already resolved"`) if the
entry is already gone. This means the underlying store call is already
race-safe for double-resolution — whichever caller (human RPC vs.
reconciliation job) wins the delete, the other gets a clean error, never a
double-send or corrupted state. **The gap is UX, not correctness**: if
reconciliation resolves an item a beat before the human's in-flight "Deny"
click reaches `ApprovalService.ResolveApproval`, that RPC will surface a
generic `CodeNotFound` — the human's explicit override attempt silently
fails with no indication the item was just auto-resolved out from under
them, and (worse) if reconciliation's decision was "allow," the human's
intended "deny" never had a chance to take effect. The plan should route
`ResolveApproval`'s not-found case through a check for "was this
auto-resolved by reconciliation" and surface that distinctly, not a generic
error.

**Reconciliation must reuse `ApprovalService.ResolveApproval`'s guard logic,
not just `ApprovalStore.Resolve()` directly.** `ResolveApproval` does more
than the store swap: it runs the CI-red block-on-approve guard
(`approval_service.go:89-113`, gated on `blockApprovalOnCIFailureFlagName` +
`liveFinder`), stamps `notificationStore.SetMetadata(...,
"approval_decision", ...)` + `MarkRead`, and broadcasts an
`ApprovalResponseEvent` over the event bus so other connected clients update
live. If reconciliation calls the lower-level `ApprovalStore.Resolve()`
directly (as a batch job naturally would, to avoid the RPC's argument
validation), it must independently reproduce all three side effects — the
CI-red guard in particular, since skipping it would let reconciliation
auto-*allow* an item a manual click on the same item would have been
blocked from approving. This is exactly the "side effects meant for live
requests" concern requirements.md's Feasibility Risks already flags, made
concrete.

**Idempotency**: naturally covered by the `Resolve()` delete-under-lock
behavior above, *provided* reconciliation always operates on a live query of
currently-pending items at run time (e.g. `ApprovalStore.ListAll()` filtered
to `Escalate`-sourced) rather than a list of IDs computed earlier and reused
across a retry — a second reconciliation pass over the same rule-change event
would simply find fewer (or zero) already-resolved items to act on.

**Blast-radius amplification — this is the core trust risk named in the
task.** Today, a classifier bug or an overly-broad new rule affects exactly
one live request at a time (whatever tool call is in flight when it's
evaluated). Reconciliation inverts that: on a single rule upsert, it
re-evaluates **every currently-pending `Escalate` item across every session**
in one pass. A bug in the new rule (or in the classifier's evaluation logic
itself — `RuleBasedClassifier.classifySingle` iterates priority-descending,
first-match-wins, `pkg/classifier/classifier.go:513-535`) that would
otherwise have caused one bad auto-approval now retroactively auto-approves
every backlog item the same over-broad pattern happens to match, all at
once, with no live human in the loop for any of them at the moment of
resolution (they escalated precisely because no rule matched at the time).
This is the scenario the visible, non-silent audit note (Observability
Requirements) is meant to make recoverable-after-the-fact, but the plan
should also consider a cheap circuit breaker — e.g. cap how many pending
items a single reconciliation pass auto-resolves before requiring
confirmation, or log a summary count prominently rather than N individually
easy-to-miss notifications — since "many things resolved that all still show
their audit note" is compliant with the letter of the requirement but not
necessarily noticed by the user in practice.

## 5. UI reboot — actionable/informational misclassification

**Confirmed live bug, not a hypothetical: `notificationTypeFilter`'s "info"
bucket is an exclusion list, and it already double-classifies an actionable
type.** `web-app/src/lib/utils/notificationMapping.ts:108-132`:

```ts
case "info":
  return types.filter(
    (t) =>
      t !== "approval_needed" &&
      t !== "auto_approved" &&
      t !== "error" &&
      t !== "task_failed" &&
      t !== "warning" &&
      t !== "task_complete"
  );
```

`"question"` (mapped from `NotificationType.INPUT_REQUIRED` —
`mapNotificationType`, line 22-23) is *not* in this exclusion list, so it
passes the "info" filter's predicate and is also returned by the
`"approval_needed"` case (`t === "approval_needed" || t === "question"`,
line 113-114). **An Input-Required notification — by definition something
needing a decision — is currently classified as belonging to both the
actionable bucket and the informational bucket.** If the IA reboot's
"Needs a decision" vs. "informational, collapsed" split reuses this helper
as-is, Input-Required items risk rendering in (or, depending on how the
reboot dedupes across buckets, being swallowed into) the collapsed group —
precisely the failure mode the task asks about. This needs a fix regardless
of the reboot; the reboot just makes the consequence of leaving it broken
visible for the first time (today "info" filter is presumably a
lower-stakes UI tab, not a collapse/hide decision).

**Fix pattern to reuse — this repo already has the precedent.**
`dynamic-rule-reload`'s ADR-002 hit the identical shape of bug in
`RulesService.rebuildClassifier()`'s rule-source filtering (`if r.Source !=
"user"` excluding one source vs. an allow-list) and explicitly rejected the
exclusion-list style for this reason: *"an exclusion filter silently keeps
[items] from any future 4th source that neither filter's exclusion list
happens to mention, while an allow-list requires every ... path to explicitly
opt a new source in."* The plan should convert `notificationTypeFilter`'s
"info" case to an explicit allow-list of genuinely-informational types
(`auto_approved`, `progress`, `reminder`, `system`, `custom`, bare `info`),
not just patch in `"question"` — a *future* new notification type (the task
already anticipates rule-reconciliation adding new event types) would
otherwise silently default into the informational bucket the same way.

**Backend side: a second, disagreeing priority source exists and is
currently dead code — do not accidentally wire it in.**
`session/queue/queue.go`'s exported `ReasonToPriority`/`DeterminePriority`
(lines 569-583) have **zero call sites outside their own definition and
`session/review_queue.go`'s re-export** (verified via repo-wide grep) — the
live `review_queue_determiner.go` assigns `Priority` inline per case instead
(`ReasonIdle`/`ReasonStale` → `PriorityLow` at determiner.go:181,294). But
`ReasonToPriority`'s switch only explicitly lists `ReasonTaskComplete`,
`ReasonIdleTimeout`, `ReasonUncommittedChanges` → `PriorityLow`; `ReasonIdle`,
`ReasonStale`, and `ReasonWaitingForUser` all fall through to
`default: return PriorityMedium` — disagreeing with the live determiner's
`PriorityLow` for exactly the two reasons (Idle, Stale) this project is
trying to demote to the collapsed tier. If the Phase 3 plan or Phase 5
implementation reaches for this well-named, exported helper (a reasonable
thing to do when building new grouping/severity logic) instead of trusting
the `Priority` value already threaded through `PendingApproval` from the
determiner, Idle/Stale items would be reclassified as Medium — landing them
in the "needs a decision" tier, the opposite failure direction from what the
task describes but the same misclassification risk class. Recommend either
deleting this dead function pair or updating it to match the live mapping
before anything new depends on it.

**`ReasonUncommittedChanges` shares the same `PriorityLow` bucket as
`ReasonTaskComplete`/idle** (`ReasonToPriority` line 578 — even though this
function is dead, the same three-way grouping is implicit in how the
determiner treats "no higher-priority reason" checks, e.g.
`review_queue_determiner.go:188,269`). "You have uncommitted changes in a
worktree" is arguably a decision item (commit? discard? review?), not purely
informational, yet it's bucketed with genuinely passive states. Worth an
explicit design call in Phase 3 (which the requirements' Open Question 3
already flags for idle specifically — extend that same question to
uncommitted-changes) rather than letting it default into "collapsed" by
inheriting Idle/TaskComplete's tier.
