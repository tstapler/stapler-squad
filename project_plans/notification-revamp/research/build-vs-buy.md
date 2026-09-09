# Research: Build vs. Buy — notification-revamp

**Scope**: six scoped fixes to an existing internal Go+React app. Default
prior is "extend what's there" — every verdict below is checked against
actual code, not assumed.

---

## 1. Existing OSS library or framework

### 1a. Backend: dedicated "event dedup with occurrence counting" library

**Current state** (`server/notifications/store.go:138-176`): `Append()`
already implements occurrence counting — `NotificationRecord.OccurrenceCount`
and `LastOccurredAt` exist as first-class fields (store.go:46-51), and the
increment/move-to-front logic is already written (store.go:157-163). The bug
(requirements Failure Mode 1) is a *scoping* bug, not a missing capability:
`findUnreadDuplicate` only matches while the existing record is unread
(per the in-file ADR-003 comment, store.go:21-27), so a read record forks a
new row instead of being reused. The fix is to broaden the duplicate lookup
to match regardless of read state and flip it back to unread on
re-occurrence — a change scoped to one function, not a new subsystem.

**Pros of a library** (e.g., an LRU/TTL dedup cache, a CRDT counter): none
apply — there's no concurrency-scale problem here (single-writer, `sync.RWMutex`
guarded, JSON-file-backed, ~500 records max per `MaxNotifications`), no
distributed dedup requirement, and no eviction-policy complexity beyond what
`enforceRetention()` already does.

**Cons**: Introducing a library here (a) doesn't remove any code — the struct
fields, the JSON persistence, and the retention/pruning logic all stay
regardless, since none of that is what a dedup library would replace; (b)
adds a dependency for what is fundamentally a 5-10 line diff to a match
condition; (c) the store's real design constraint (JSON-file persistence, no
new persistence layer per the requirements' Constraints section) rules out
most dedup libraries anyway, which assume an in-memory or Redis-backed cache.

**Verdict: Not recommended.** Fix `findUnreadDuplicate`'s scope condition
in place. This is a 1-line-of-reasoning bug fix, not a capability gap.

### 1b. Frontend: grouped/collapsible notification list library

**Current state**: `web-app/package.json` already depends on
`@radix-ui/react-accordion` (^1.2.17), and the repo has already built a
purpose-made wrapper around it: `web-app/src/components/ui/Collapsible.tsx`
exports `CollapsibleGroup`/`CollapsibleSection`, explicitly designed for
"a group of sibling sections" sharing one accordion root with correct
roving-tabindex keyboard behavior (ADR-027, referenced in the file's own
header comment). It's already used in three unrelated features —
`BacklogItemDetail.tsx`, `WorkflowsPanel.tsx`, and `VcsWidgetComments.tsx` —
so it's a proven, established pattern in this exact codebase, not a
one-off. `@tanstack/react-virtual` and `react-virtuoso` are also already
dependencies, available if the collapsed/grouped list ever needs
virtualization for long session histories (not obviously needed at "dozens
of sessions" scale per the requirements' Non-functional Requirements).

Grouping notifications by session is a plain array `reduce`/`Map` — no
library exists in package.json for this (no lodash), and none is needed;
it's a ~10-line data transform, not infrastructure.

**Pros of building a new grouping component**: none identified — it would
duplicate `Collapsible.tsx`'s accessibility work (keyboard nav across
sibling headers) that's already solved and tested.

**Verdict: Recommended (reuse, don't build or add).** Notifications page IA
reboot should compose `CollapsibleGroup`/`CollapsibleSection` from
`web-app/src/components/ui/Collapsible.tsx` for the "recent/completed
activity grouped per-session and collapsed" requirement, with a plain
in-component `groupBy(sessionId)` reduce for the grouping itself. Same
applies to the Review Queue page's priority-tier grouping.

---

## 2. SaaS / managed API (PagerDuty/Opsgenie-class alert dedup)

**Verdict: Not recommended.** Brief justification, not an exhaustive
vendor comparison, per the requirements' own framing:

- **No existing integration surface.** This is a single-operator internal
  tool (`~/.stapler-squad/`, JSON-file config, no external alerting
  pipeline) — there's no on-call rotation, no escalation policy, no
  multi-channel routing problem that PagerDuty/Opsgenie exist to solve.
- **Wrong shape of problem.** The actual bugs (Failure Modes 1-3 in
  requirements.md) are in-process state bugs — a dedup key scoped too
  narrowly, a missing acknowledgment-suppression flag, a missing
  reconciliation trigger. None of these are "how do we route this alert to
  the right human across Slack/SMS/phone" problems; routing to a managed
  alerting SaaS wouldn't touch any of the three root causes.
- **Cost/latency/complexity mismatch.** Adding a network dependency (API
  keys, webhook plumbing, a new external outage mode) for a tool whose
  Non-functional Requirements explicitly say "single-operator... not a
  driver of design decisions" and "no new persistence layer... unless
  unavoidable" is disproportionate. It would also violate the "No feature
  flag... internal tool behind no public traffic" Risk Control framing by
  adding a public-internet dependency where none currently exists.

---

## 3. LLM-generated implementation vs. battle-tested code path (rule-reconciliation)

**Question**: should rule-reconciliation reuse the exact same `Classify()`
call path as live requests, rather than new reconciliation-specific logic?

**Verdict: Recommended — reuse `Classify()` directly.** Verified this is
mechanically straightforward, not just theoretically nice:

- `RuleBasedClassifier.Classify(payload PermissionRequestPayload, ctx
  ClassificationContext) ClassificationResult` (`pkg/classifier/classifier.go:456`)
  is the single decision function every live request already goes through.
- The stored `PendingApproval` record
  (`server/services/approval_store.go:21-49`) already retains every field
  needed to reconstruct a `PermissionRequestPayload`: `ToolName`,
  `ToolInput map[string]interface{}`, `Cwd`, `PermissionMode`, plus
  `SessionID`. `ClassificationContext`'s remaining fields (`IsGitRepo`,
  `RepoRoot`, `IsWorktree`, `CIStatus`, `SessionIdleMinutes`) are exactly
  the same derived-from-session-state fields `approval_handler.go:370-374`
  already computes for live requests — reconciliation can call the same
  helper rather than inventing parallel derivation logic.
- This means reconciliation is "for each pending Escalate-created item,
  rebuild the payload/context from stored fields, call the *same*
  `Classify()`, and act only if the decision is no longer `Escalate`" — a
  thin driver around existing, already-tested logic, not new business
  rules.

**Why this specifically avoids the feasibility risk called out in
requirements.md** ("re-run the classifier against pending items without
re-triggering side effects meant for live requests, e.g. no
double-notification, no interference with a request the user is mid-way
through answering"): reusing `Classify()` itself is side-effect-free — it's
a pure decision function (`ClassificationResult`, no I/O). The side effects
requirements worries about live entirely in the *caller* — the
`switch result.Decision` block in `approval_handler.go:388-419` that writes
`h.autoApprovalLog`, calls `h.writeDecision(w, ...)` against an HTTP
response writer, etc. Reconciliation must **not** reuse that switch; it needs
its own thin caller that only handles the two now-relevant outcomes
(`AutoAllow`/`AutoDeny` → resolve + audit note per the Observability
Requirements; anything else → leave untouched) and never touches
`h.writeDecision` (no live HTTP connection exists for an orphaned/pending
item) or double-fires `AppendAutoApproved`-style notifications without the
required "Auto-resolved by rule: `<name>`" audit framing from the Success
Metrics section.

**An LLM writing bespoke reconciliation classification logic from scratch**
(re-deriving tool-pattern matching, risk levels, etc.) would be exactly the
failure mode requirements.md warns about ("exactly the kind of bespoke
business logic an LLM might implement subtly wrong") — it would drift from
the live path's behavior over time (e.g., a future rule-matching edge case
fixed in `Classify()` wouldn't automatically apply to reconciliation) and
duplicate ~2,850 lines of classifier logic (`pkg/classifier/classifier.go`)
that's already covered by its own test suite. Reuse is both less code and
higher correctness confidence — no tradeoff here.

---

## 4. Fork or adapt: salvage from the three stalled plans

Read all three `implementation/plan.md` files in full. Verdict per plan:

### `review-queue-state-detection` (2026-05-02, 28 tasks, never implemented)

**Salvageable, directly relevant (Epic 1 — Detection Correctness):**
- Epic 1.1 (`DetectRecent` tail-limiting), 1.2 (CR-collapsing for spinner
  sequences), 1.3 (missing patterns: cost-summary line, readline prompt,
  thinking-verb regex), 1.4 (unify `detectProcessing()` onto the same
  `DetectRecent` path) are narrow, low-risk detector fixes that directly
  serve this project's Scope item 3 ("prevent sessions actively producing
  output/mid-turn from being classified idle/stale"). These tasks are
  self-contained, already-designed, and don't depend on the plan's later
  phases — genuinely adoptable as-is, pending a check that the referenced
  line numbers/patterns still match current `session/detection/detector.go`
  (the plan is 4 months old).
- **Confirms Open Question 1's answer directly**: this plan's own framing
  (and this research's reading of current code) shows the "detection
  package's `StatusIdle`/`StatusContext` state machine" is not a separate,
  incompatible system built only for unattended-PTY-write gating — it's the
  same `session/detection.IdleDetector`/`IdleState` machinery already
  wired into `review_queue_determiner.go`'s controller-active branch
  (`statusInfo.IdleState.State`, `review_queue_determiner.go:126-184`,
  reading `IdleStateActive`/`IdleStateWaiting`/`IdleStateTimeout`). The real
  gap is narrower than the open question implies: the **no-controller**
  branch of `Determine()` (`review_queue_determiner.go:195-266`) bypasses
  `IdleDetector` entirely and uses a bare `time.Since(inst.UpdatedAt) >
  basicIdleThreshold` (5s) check instead. So: **reuse is not just viable,
  it's already half-done** — this project's working-state-detection scope
  item should extend the no-controller path to run content through
  `IdleDetector`/`DetectRecent` the same way the controller path already
  does, rather than inventing parallel logic.

**Not salvageable / explicitly descoped:**
- Epic 2 (push-based `WorkingState` proto enum, `ClaudeController` status-
  change callbacks) and Epic 3 (frontend filter/count UI keyed off that
  enum) — this is the `review-queue-event-driven` latency work under a
  different name; requirements.md explicitly puts the poller rearchitecture
  Out of Scope. Building the proto enum just to satisfy Scope item 3 would
  be over-engineering: the no-controller path already has direct access to
  `IdleState` via the same `detector`/`IdleDetector` the controller path
  uses — no new proto field is needed to fix the misclassification itself,
  only to *expose* the state to the frontend, which isn't in this project's
  scope.
- Epic 4 (golden-state corpus tooling, `CaptureStateSnapshot` RPC,
  `make corpus-validate`) — requirements.md explicitly calls this out as
  Out of Scope ("disproportionate infra for a single-operator tool").

### `review-queue-event-driven` (2026-05-09, 21 tasks, never implemented)

**Not salvageable for this project — correctly Out of Scope.** This plan
replaces the poller's 2s cadence with PTY-output-driven push events
(`statusCheckCh`, `ClaudeController.SetStatusChangeListener`,
`ReactiveQueueManager.OnControllerStatusChange`). It's a latency
optimization (~2s → ~1s) fully orthogonal to the four failure modes this
project fixes — none of the notification-dedup, idle-suppression,
reconciliation, or IA-reboot scope items need faster polling, they need
*correct* classification and *persistent* acknowledgment of what's already
being polled at 2s intervals. Adopting any part of this plan (even
piecemeal, e.g. just the `cacheMu` race fix in Task 1.1.0) would be scope
creep unless a race is independently discovered during Scope item 3's
work — worth flagging to implementation, not pre-emptively pulling in.

### `smart-notification-dedup` (2026-05-29, 7 files, never implemented)

**Different subsystem, correctly separated by requirements.md's Out of
Scope.** This plan's dedup target is `session/tmux/fork_metrics.go`'s
health-alert `checkPressure()` (fork-pressure/zombie-count conditions) and
the frontend's native OS `Notification` lifecycle
(`web-app/src/lib/utils/notifications.ts`) — entirely distinct from
`server/notifications/store.go`'s `NotificationHistoryStore.Append()`,
which is this project's Failure Mode 1. No code here is directly reusable
for the in-scope dedup fix (different struct, different store, different
trigger condition — condition-change gating on a metrics struct vs.
read-state gating on a JSON-backed record store).

**Conditionally salvageable if appetite allows** (requirements.md: "include
only if appetite allows after the core fixes"): Epic 2's native-notification
auto-close/close-before-open pattern
(`activeNativeNotifications: Map<string, Notification>`, tag-based
close-before-open, `setTimeout`-based auto-close) is a clean, self-contained,
already-designed frontend change with its own Jest test plan
(Story 2.2.3) — if Phase 3 planning finds room after the core fixes, this
is the one piece of the three stalled plans that's a genuine drop-in
adopt-as-written candidate, not a rethink. Its Epic 1 (Go `forkMonitor`
condition-change gating) is independent of the frontend piece and could be
landed alone even smaller-scoped.

---

## Summary of verdicts

| Question | Verdict |
|---|---|
| Go library for dedup/occurrence-counting | **Not recommended** — existing `OccurrenceCount` fields already do this; fix is a duplicate-matching scope bug, not a missing capability |
| Frontend grouping/collapsible library | **Recommended: reuse in-repo `Collapsible.tsx`** (already wraps `@radix-ui/react-accordion`, used in 3 other features) — no new dependency |
| SaaS alert-dedup service | **Not recommended** — wrong problem shape, no integration surface, violates the tool's no-public-traffic risk posture |
| Reuse `Classify()` for reconciliation vs. new logic | **Recommended: reuse `Classify()` directly** — `PendingApproval` already stores everything needed to rebuild the payload; the function itself is side-effect-free, so reuse also directly defuses the feasibility risk about re-triggering live-request side effects |
| Fork `review-queue-state-detection` | **Partially salvage: Epic 1 (detector fixes) adoptable as-is; Epics 2-4 correctly out of scope** |
| Fork `review-queue-event-driven` | **Not salvageable — correctly out of scope**, orthogonal latency concern |
| Fork `smart-notification-dedup` | **Different subsystem, not salvageable for core scope; Epic 2 (native notification lifecycle) is a clean drop-in candidate if appetite allows** |
