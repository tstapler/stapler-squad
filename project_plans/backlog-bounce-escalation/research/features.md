# Research: Feature Landscape — backlog-bounce-escalation

## 1. Industry patterns

### Flaky-test quarantine (CI/testing)
- GitHub Actions has no native flake-quarantine primitive — teams bolt on third-party tools
  (Trunk Flaky Tests, BuildPulse, Cypress Cloud) that auto-score tests by re-run history and
  **move a flaky test to a separate, non-blocking run** so the main pipeline stays green while
  the flaky test's results are still tracked, not silently dropped. [Flaky Test Quarantine in
  GitHub Actions](https://tenki.cloud/blog/flaky-test-quarantine-github-actions), [Best Tools to
  Fight Flaky Tests in CI/CD Pipelines (2026)](https://www.shiplight.ai/blog/best-tools-flaky-tests-ci-cd)
- The core mechanism transfers directly: **classification changes the gate, not the outcome**.
  A flaky-classified change isn't exempted from review — it's routed through a stricter or
  different pass criterion (repeated-run confirmation) instead of a single pass/fail check.
  Relevant to requirement item 3 (differentiated review strategy): the analogous move here is
  requiring N consecutive non-flaky verdicts rather than a single PASS, not lowering the bar.
- GitHub's own workflow-rerun cap (50 reruns) is a reminder that "just retry more" has a hard
  ceiling — reinforces the requirements doc's explicit non-goal of raising
  `MaxRemediationAttempts`.

### Escalation policies (PagerDuty)
- PagerDuty ties severity to the *alert*, and an escalation policy is fixed per service — to
  split by severity you need **separate services/policies**, not a single policy that
  re-evaluates severity live. [Escalation Policy Basics](https://support.pagerduty.com/main/docs/escalation-policies),
  [PagerDuty Services Example: Split High and Low Severity Alerts to Different Escalation
  Policies](https://community.ops.io/pdcommunity/pagerduty-services-example-split-high-and-low-severity-alerts-to-different-escalation-policies-1mnn)
  — informs a design choice: don't try to make one notification path dynamically "get louder";
  model the elevated case as a structurally distinct signal (own DB row / own notification
  type), same shape as this repo's existing per-reason `backlog_stuck_states` rows.
- Multi-user alerting lets multiple people be notified **at the same escalation level**
  simultaneously. [Alert All the Right People with Multi-User
  Alerting](https://www.pagerduty.com/blog/announcements/alert-all-the-right-people-with-multi-user-alerting/)
  — the closest analogue to "N simultaneous stuck reasons" is closer to *co-occurring alerts on
  one incident* than to a single alert's severity escalating; PagerDuty's answer for that case is
  incident correlation/grouping (multiple alerts collapse into one incident with an aggregate
  view), which maps well onto "compute an item-level severity from N per-reason rows" rather than
  inventing a new reason.

### SLA breach escalation (Jira Service Management)
- The dominant pattern is **staged, threshold-based**: proactive warning before breach (e.g. at
  85% of SLA time), then an automated action at breach — reassign, notify a different audience,
  or move to a distinct status like "Needs Attention"/"Escalated" so urgency is visible outside
  the normal queue view. [JSM Jira Automation: How to Send SLA Breached
  Notifications](https://community.atlassian.com/forums/Jira-Service-Management-articles/JSM-Jira-Automation-How-to-Send-SLA-Breached-Notifications/ba-p/1894625),
  [The inevitable SLA Breach in Jira: 5 Steps from panic to a
  plan](https://community.atlassian.com/forums/App-Central-articles/The-inevitable-SLA-Breach-in-Jira-5-Steps-from-panic-to-a-plan/ba-p/3159740)
- Explicit anti-pattern named in the sources: sending *every* SLA alert to leadership causes
  alert fatigue — filter to what's actually escalation-worthy.
  [source](https://community.atlassian.com/forums/App-Central-articles/The-inevitable-SLA-Breach-in-Jira-5-Steps-from-panic-to-a-plan/ba-p/3159740)
  Directly relevant: this repo is single-user, so "who gets notified" isn't the axis, but "how
  many separate toasts fire" is — the requirements doc's existing gap (one-time toast, not
  durable) is exactly what Jira's "move to a distinct persistent status" pattern fixes.
- Jira's breach state is itself durable (a status/flag on the ticket, not just an outbound
  notification) — matches this project's requirement for a **DB-backed marker**, not a
  point-in-time event.

### Synthesis — what these three domains agree on
1. Escalation/quarantine state must be a **durable, queryable marker**, not just a fired
   notification (Jira SLA-breach status, CI quarantine list). A toast alone is what this project
   is explicitly trying to replace.
2. **Severity aggregation happens at the correlation layer**, not by mutating a single alert's
   urgency in place (PagerDuty incident grouping). Maps to "compute a derived item-level signal
   from the existing per-reason rows" rather than writing a new field onto each reason row.
3. **Classification changes the verification bar, not the outcome** (CI quarantine moves a test
   off the blocking path but still tracks it; it doesn't mark it passing). For flaky-test-aware
   review, the analogous design is a stricter/different verdict requirement, not a bypass.

## 2. Edge cases and failure modes to handle

Derived from reading the actual reconciliation code (see Section 3) plus the industry patterns
above:

- **Item resolves between detection and notification.** `RemediationDue` performs its
  eligibility check and its write (`RecordRemediationAttempt`) essentially atomically inside one
  DB call (`session/backlog_remediation.go:168-193`), but the *notify* call happens synchronously
  right after, outside any check that the underlying condition still holds. A multi-reason
  severity signal computed by re-querying `FindOpenStuckStates` at notify-time (rather than
  caching a count from an earlier tick) avoids stale escalation, but any durable "escalated"
  marker must still have an explicit resolution path (mirroring `ResolveStuck`) — see the next
  point.
- **Flapping: reasons opening/closing within the same reconcile cycle.** Each reason already
  gets its own idempotent `MarkStuck`/`ResolveStuck` pair per detector tick (confirmed:
  `MarkStuck` is called repeatedly and is a no-op update when the row is already open —
  `session/ent_repository_backlog.go:1199`). A naive "count of currently open reasons" read at
  each tick would flap an item's severity in and out of "escalated" every 60s if a reason is
  right at its detection threshold boundary. Needs either (a) a minimum-duration-at-elevated-count
  gate (mirroring `abandonedReviewGrace`'s 15-minute grace before flagging), or (b) hysteresis
  (escalate at ≥2, de-escalate only at 0, not immediately at <2) so a single reason resolving
  doesn't silently flip a 3-reason item back to "normal" for one tick then re-escalate.
- **An item that will ALWAYS have 2 reasons "by design."** Some reason pairs may be structurally
  coupled rather than independently informative — e.g. `bouncing` (rework cycling without a
  PASS) and `abandoned_review` (no active review session) can legitimately co-occur any time a
  bouncing item's review gate happens to be between spawns, without indicating anything worse
  than a single-reason case. A raw count threshold (`≥2`) risks false-escalating this
  structurally-common pair on every bouncing item, diluting the signal for genuinely novel
  3+-reason combinations. The requirements doc's own "keep it a simple count/threshold, not a
  tunable rubric" constraint conflicts with this risk — planning should explicitly check, against
  the current live data, whether `bouncing`+`abandoned_review` co-occurs so often that a plain
  count would fire constantly (in which case a defensible mitigation is: count only reasons that
  are NOT already implied by another open reason, or require the count to include at least one
  "non-progress" reason like `autonomous_stuck`/`orphaned_triage` beyond the bouncing/review pair).
- **Race between the two escalation triggers (multi-reason vs. capped-while-bouncing).** These
  are two independent conditions (item 1 and item 2 in the requirements). An item can satisfy
  both simultaneously (e.g. `92d679fd`-shaped: capped bouncing attempts AND a second open
  reason). The design needs one severity/marker model that can represent "why" precisely — not
  two separate boolean flags whose combination isn't itself surfaced — or a user reading the UI
  can't tell which specific condition(s) triggered escalation.
- **Notification dedup vs. re-escalation.** The existing `MarkStuckNotified` pattern
  (`session/ent_repository_backlog.go:1294`, `session/storage.go:969`) exists to send a
  per-reason notification exactly once. A new durable escalation marker needs an equivalent
  "notified" bit — but also needs to decide whether re-opening after resolution (item recovers,
  then re-escalates later) should re-notify. Silent non-renotification risks the same "one-time
  toast, user has to know to look" gap the requirements doc is trying to close.
- **Snoozed reasons should not count toward the multi-reason total.** `FindOpenStuckStates`
  already filters out snoozed rows (`session/storage_backlog.go:912-916`) — good, this is free —
  but if severity computation ever reads stuck rows through a different path (e.g. a raw DB
  query added for a new RPC), it must apply the same snooze filter or a user who intentionally
  snoozed one reason will still see the item escalate from the remaining reasons in a way that
  contradicts their explicit "don't bug me about this yet" action.
- **Restart-grace interaction.** `evaluateRemediation`'s restart-grace path
  (`session/backlog_remediation.go:104-109`) exists specifically because an OOM-restart storm
  previously caused false attempt-cap parking. A "capped while bouncing" escalation trigger must
  key off `RemediationAttempts >= MaxRemediationAttempts` (the parked state), not off raw
  attempt-count velocity, or a restart storm could trip the new escalation signal for the same
  false-positive reason the grace period was built to prevent.
- **Flaky classification heuristic false-positive/negative.** The requirements doc's own "Rabbit
  Holes" section flags this: a keyword heuristic ("flaky", "-race", "intermittent" in
  title/description) will both under- and over-match. An edge case worth naming explicitly: an
  item whose *title* doesn't mention flakiness but whose review verdicts repeatedly show
  `IsRepeatedFailure`-style identical-summary failures (`session/stuck_decisions.go:85-93`) is
  arguably a stronger flaky signal than a keyword match — worth flagging to planning as a
  candidate heuristic ("same review verdict repeated" already exists as machinery) even if full
  classification is out of scope.

## 3. Unstated needs beyond the explicit requirements

- **A single place to see "why is this item escalated" that composes existing reason rows**,
  not a new parallel explanation. The requirements doc frames this as UI/notification work, but
  the underlying need is: today `ListStuckBacklogItems` returns one row per (item, reason) —
  there is no item-level rollup RPC at all. Any UI severity badge needs either a client-side
  group-by (fragile, duplicates backend logic) or a new aggregate field/RPC — this is implied by
  "distinguished... without requiring the user to cross-reference the stuck-state table by hand"
  but not spelled out as a data-shape requirement.
- **Consistency with the existing `docs/tasks/backlog-stuck-item-auto-remediation.md` design
  language.** That doc's phase structure (Phase A backoff, later phases) and this repo's own
  precedent (`backlog-stuck-item-visibility`'s explicit "visibility, not control panel" scope
  decision, cited directly in this requirements doc) suggest the user expects escalation output
  to slot into the *same* mental model (durable rows, snooze/reset affordances) rather than
  introduce a stylistically different subsystem. An implementation that doesn't reuse
  `SnoozeStuckItem`/`ResetStuckRemediation`-shaped affordances for the new escalation marker
  would feel inconsistent even though nothing in the explicit requirements mandates it.
- **A verification story that survives the three named items resolving.** The requirements doc
  already names this as a Feasibility Risk, but the unstated need is sharper: because this is a
  single-developer, self-verified project, the "Success Metrics" verification step needs to be
  re-runnable against *whatever* items are live at implementation/verify time, which argues for
  a test fixture that synthesizes the 2-reason and capped-while-bouncing conditions directly
  (table-driven, matching `evaluateRemediation`'s/`isBouncing`'s existing pure-function test
  style) rather than a manual DB inspection step that only works today.
- **Avoiding notification fatigue on a single-user instance.** Nothing in the requirements
  mentions rate-limiting the new escalation notification, but the SLA-breach research above
  names alert fatigue as a named anti-pattern, and this codebase already fights a related
  problem (`autoReopenWithBackoffGate`'s entire existence is to stop rapid-fire re-attempts).
  A multi-reason count that flaps at a boundary (see Edge Cases above) without a
  once-per-escalation-episode notify gate would reintroduce the exact toast-spam problem this
  project is meant to fix, just one level up.

## 4. Codebase orientation (as read, not exhaustive)

- `server/services/backlog_service_stuck.go` — RPC surface: `ListStuckBacklogItems` (read,
  returns one `StuckBacklogItem` per open+un-snoozed reason row),
  `SnoozeStuckItem`/`ResetStuckRemediation`/`BulkResetStuckRemediation` (operator actions),
  `TriggerRemediationNow` (manual retry, still consumes attempt budget, still respects the
  parked-cap and each action's own circuit breaker). `remediationActionByReason` is the
  reason→respawn-function dispatch table — any new reason needing a manual-trigger action would
  register here. No existing "item-level" (as opposed to per-reason) aggregation RPC.
- `session/stuck_decisions.go` — pure, DB-independent detector predicates
  (`stuckPRReady`, `abandonedReview`, `staleWork`, `isBouncing`,
  `IsRepeatedFailure`/`IsRepeatedNoVerdictFailure` for non-converging review cycles,
  `prReadyToMergeSolo`). Deliberately kept side-effect-free and exhaustively table-driven-tested
  — the doc comment at the top of the file explains this pattern exists specifically so
  threshold/cycle arithmetic (this project's "N simultaneous reasons" threshold logic
  included) doesn't need a DB to test.
- `session/backlog_remediation.go` — the shared exponential-backoff gate
  (`RemediationDue`/`RemediationBlocked`/`evaluateRemediation`) all automated remediation goes
  through. `MaxRemediationAttempts = int32(len(remediationBackoffSchedule)) = 5`. Parking is
  purely `RemediationAttempts >= MaxRemediationAttempts`, independent of backoff timing — this is
  the exact condition requirement item 2 ("capped while bouncing") needs to detect, already
  computed as `justParked` at the two call sites below.
- `session/backlog_lifecycle_review.go:194-223` (`autoReopenWithBackoffGate`, the `bouncing`
  reason's gate) and `:576-681` (`markAbandonedReview`, the `abandoned_review` reason's gate) —
  both call `RemediationDue`, and both fire the existing one-time "Auto-rework paused" /
  "N times... use Reset" notification via `l.notify(...)` exactly when `justParked` is true
  (lines 206-211 and 658-663 respectively — the exact call sites the requirements doc cites at
  ":208,660"). Both notifications are structurally identical strings differing only in "rework"
  vs. "re-review" — confirms requirement item 2's claim that today's cap-out notification is
  generic and doesn't distinguish "capped while a `bouncing` reason is *also* still open" from an
  ordinary single-reason park. A new escalation path most naturally slots in as an additional
  branch inside these same `justParked` blocks (or a shared helper both call), checking whether
  `domain.StuckReasonBouncing` has an open row via `FindOpenStuckStates`/`RemediationBlocked`
  before deciding whether to emit the elevated signal instead of (or in addition to) the existing
  toast.
- `session/storage_backlog.go:894-910` — `OpenStuckStateData` is the per-reason row shape
  (`ItemID`, `Reason`, `RemediationAttempts`, `NextRemediationAt`, etc.). `FindOpenStuckStates`
  already excludes resolved and snoozed rows, so grouping its results by `ItemID` client-side
  (in a new service-layer function, not a new detector) is the natural way to compute "how many
  simultaneous open reasons does this item have right now" without touching the schema —
  consistent with the requirements doc's constraint to build on existing infrastructure rather
  than parallel tracking.
- No existing flaky-test classification code anywhere in `session/` or `server/services/`
  (confirmed via grep for "flaky"/"non-deterministic"/"intermittent" — zero non-test hits).
  Requirement item 3 is greenfield within this codebase; the closest existing machinery is
  `IsRepeatedFailure`/`IsRepeatedNoVerdictFailure`'s "identical outcome across attempts" signal,
  which is a plausible cheap proxy worth flagging to planning (see Edge Cases above).

## Sources

- [Flaky Test Quarantine in GitHub Actions](https://tenki.cloud/blog/flaky-test-quarantine-github-actions)
- [Best Tools to Fight Flaky Tests in CI/CD Pipelines (2026)](https://www.shiplight.ai/blog/best-tools-flaky-tests-ci-cd)
- [Escalation Policy Basics — PagerDuty](https://support.pagerduty.com/main/docs/escalation-policies)
- [PagerDuty Services Example: Split High and Low Severity Alerts to Different Escalation Policies](https://community.ops.io/pdcommunity/pagerduty-services-example-split-high-and-low-severity-alerts-to-different-escalation-policies-1mnn)
- [Alert All the Right People with Multi-User Alerting — PagerDuty](https://www.pagerduty.com/blog/announcements/alert-all-the-right-people-with-multi-user-alerting/)
- [JSM Jira Automation: How to Send SLA Breached Notifications](https://community.atlassian.com/forums/Jira-Service-Management-articles/JSM-Jira-Automation-How-to-Send-SLA-Breached-Notifications/ba-p/1894625)
- [The inevitable SLA Breach in Jira: 5 Steps from panic to a plan](https://community.atlassian.com/forums/App-Central-articles/The-inevitable-SLA-Breach-in-Jira-5-Steps-from-panic-to-a-plan/ba-p/3159740)
