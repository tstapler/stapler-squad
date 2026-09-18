# Architecture Research: backlog-bounce-escalation

Builds directly on `project_plans/backlog-stuck-item-visibility/research/architecture.md`
(hereafter "prior research"). That project's recommended design (§3, "hybrid — derive at
query time, persist only the notify-once dedup key") has since been **built and shipped**:
`session/ent/schema/backlog_stuck_state.go` implements exactly the "option B" resolve-in-place
table it proposed, and `session/backlog_lifecycle.go`'s `ReconcileStuck` (prior research §5,
confirmed still the sole ticker call site — `server/dependencies.go:1076`) now runs ~13
panic-isolated detectors (`runStuckDetector`, `session/backlog_lifecycle.go:869-879`) that each
`MarkStuck`/`ResolveStuck` a `domain.StuckReason` row. This research does not re-derive any of
that; it identifies exactly where the two new signals (multi-reason accumulation, cap-hit-while-
bouncing) plug into the now-existing machinery.

## 1. What actually exists today (supersedes prior research §1, §3 as "recommended")

- `session/domain/backlog.go:36-150` — `StuckReason` is a validated string enum with 14
  current values (`AllStuckReasons`, line 153), including `StuckReasonBouncing` (line 53).
- `session/ent/schema/backlog_stuck_state.go` — one row per `(item_id, reason)`, unique index
  enforced (line 90), resolve-in-place (not append-only — deliberately, see the schema's own
  doc comment and ADR-001 in that project's `decisions/`). Fields directly relevant here:
  `remediation_attempts int32` (line 59), `next_remediation_at *time.Time` (line 62),
  `notified_at *time.Time` (line 44), `resolved_at *time.Time` (line 48).
- `session/backlog_remediation.go` — the shared exponential-backoff gate. `MaxRemediationAttempts`
  (line 45) = 5 (`len(remediationBackoffSchedule)`, NOT 3 as prior research's era assumed — the
  cap was raised and decoupled from `server/services/backlog_service_triage.go`'s
  `maxAutoReworkIterations`, which is a *separate*, older constant gating a different thing:
  how many rework cycles `AutoReopenAfterFailedReview` will spawn, not how many times the
  *stuck-row* remediation gate will fire). `evaluateRemediation` (line 96) is the pure decision
  function; `RemediationDue` (line 168) is the DB-integrated gate every remediation action call
  site shares.
- `server/services/backlog_service_stuck.go` — the full RPC surface already exists:
  `ListStuckBacklogItems` (line 130, `+api: backlog:list-stuck`), `SnoozeStuckItem`,
  `ResetStuckRemediation`, `BulkResetStuckRemediation`, `TriggerRemediationNow`. The frontend
  (`web-app/src/components/backlog-stuck/*`, `web-app/src/lib/hooks/useStuckBacklogItems.ts`)
  already renders these rows.

**Implication for this project**: requirements.md's constraint ("must build on the existing
`backlog_stuck_states` schema/detectors... rather than introducing parallel tracking") is not
just a preference — the schema, RPC, and UI plumbing for "a row = a durable, queryable,
notify-once, snoozable, resettable stuck condition" is 100% present. The cheapest, most
consistent implementation of *both* new signals is to represent each as its own
`domain.StuckReason` value and reuse `MarkStuck`/`ResolveStuck`/`MarkStuckNotified` verbatim —
not a new table, not new proto messages, not a new frontend component family.

## 2. Signal 1: count of simultaneously open stuck reasons per item

**Answer: query-time aggregation, computed from the existing `FindOpenStuckStates` result set —
no new persisted count field.**

`session/storage_backlog.go:917-957`'s `FindOpenStuckStates` already returns every open,
un-snoozed `BacklogStuckState` row across all items in one query (`resolved_at IS NULL AND
(snoozed_until IS NULL OR snoozed_until < now)`), each carrying `ItemID` and `Reason`. Grouping
that slice by `ItemID` and counting is an O(n) in-memory operation over a result set that is
already fetched once per reconcile tick (several existing detectors call it, e.g.
`reconcileStaleWorkSessions` at `session/backlog_lifecycle_stale.go:114`, `selfHealStuck` at
`session/backlog_lifecycle.go:1629`). No new ent field, no new index, no migration is needed to
*compute* the count.

What **is** needed is a durable place to record "this item has already been notified/escalated
for N-simultaneous-reasons" so the 60s ticker doesn't re-notify every tick once escalated (the
same notify-once problem every existing detector solves via `notified_at`). Two ways to get
that without inventing new storage:

- **Recommended**: treat "multi-reason escalation" itself as a new synthetic `StuckReason`
  (e.g. `StuckReasonMultipleReasons`), `MarkStuck`'d against the item once its open-reason count
  crosses the threshold, using `item.Status` as `expectedStatus` (both `in_progress` and
  `review` are valid anchor statuses for the underlying reasons that can co-occur, so the
  detector passes whatever the item's actual current status is — `MarkStuck`'s precondition
  check, `session/ent_repository_backlog.go:1215`, only requires `current.Status ==
  expectedStatus`, it does not hardcode which status). This reuses `notified_at` for
  notify-once dedup and `resolved_at` for auto-clear once the count drops back below threshold
  (via the same `ResolveStuck` call every other detector uses) — for free, it also shows up in
  `ListStuckBacklogItems` and is snoozable/resettable through the existing RPCs with zero new
  frontend work beyond a label/severity mapping for the new reason string.

## 3. Signal 2: hit `MaxRemediationAttempts` while `bouncing` reason still open

**Answer: this is already computed as a side effect of the existing gate — no new detection
logic needed, only a new durable marker for the *outcome*.**

`session/backlog_remediation.go:168` `RemediationDue` already returns `justParked=true` at the
exact moment `remediation_attempts` reaches `MaxRemediationAttempts` (line 191: `nextAttempt >=
MaxRemediationAttempts`), for whichever `(itemID, reason)` pair was gated. The bouncing-specific
call site is `session/backlog_lifecycle_review.go:194-223`'s `autoReopenWithBackoffGate`, which
today just fires the generic "use Reset to try again" notification (line 206-211) and does
**nothing durable** — this is precisely requirements.md's baseline gap #2. The row itself
*already* durably encodes "parked while bouncing": `remediation_attempts >= 5 AND
next_remediation_at IS NULL AND reason == "bouncing" AND resolved_at IS NULL` is a pure
predicate over columns that already exist (`session/ent_repository_backlog.go:1436-1445`
already has a near-identical `RemediationAttemptsGTE(MaxRemediationAttempts)` predicate for a
different purpose — the "parked" bulk-reset filter — confirming this exact condition is already
queryable with zero schema change).

So: no new detection is needed at all for *whether* this happened — `justParked` at line 205 of
`backlog_lifecycle_review.go` already tells you precisely. What's missing is only that this
event's *consequence* isn't durable (matches requirements.md's success metric: "a signal that
persists (queryable) rather than being a one-time toast"). Recommended: at the `if justParked`
branch (line 205), in addition to (or instead of) the existing generic `l.notify(...)` call,
also `MarkStuck` a new synthetic reason (e.g. `StuckReasonBounceCapExhausted`) anchored at
`BacklogStatusInProgress` (the status `bouncing` rows are anchored at — see `selfHealStuck`'s
switch, `session/backlog_lifecycle.go:1650-1651`). This row persists until `bouncing` itself
resolves (PR merges, item ships, or an operator resets), giving exactly the durable,
distinguishable signal requirements.md asks for, using the existing infrastructure end to end.

## 4. Race conditions specific to these two signals (extends prior research §4)

1. **Escalation count read is a point-in-time snapshot across N independent detector writes in
   the same tick.** `runStuckDetector` runs `stale_work`, `abandoned_review`, `bouncing`, etc.
   sequentially within one `ReconcileStuck` call (no concurrency within a tick — prior research
   §4.4 already established the ticker itself is a single goroutine), so as long as the
   multi-reason escalation detector is registered **after** every reason-specific detector in
   the `ReconcileStuck` body (i.e., near `self_heal` at `session/backlog_lifecycle.go:1061`, not
   before), the count it reads via `FindOpenStuckStates` reflects this tick's fully-settled
   state, not a stale mid-tick partial view. Registering it before the others would under-count
   by exactly the not-yet-run detectors' contributions that tick — an easy, easy-to-miss
   ordering bug.
2. **`selfHealStuck`'s per-reason switch must be extended.** `session/backlog_lifecycle.go:1643-`
   is an explicit `switch row.Reason` with one case per existing reason, defaulting (implicitly,
   via the blanket terminal-status rule at line 1635) to "never auto-resolve" for any reason not
   listed. A new synthetic reason added without a matching `case` here will never self-heal on
   status change (only the escalation detector's own next-tick "count dropped below threshold"
   resolve, or a terminal done/archived transition, would close it) — decide explicitly whether
   that's acceptable (probably yes for `StuckReasonMultipleReasons`, since its own detector is
   the natural resolver) or add a case.
3. **`MarkStuck`'s `expectedStatus` precondition can silently no-op the escalation write.** If
   the escalation detector computes "3 open reasons, item.Status == in_progress" but by the time
   it calls `MarkStuck` the item has since transitioned (e.g. a concurrent `AutoReopenAfterFailedReview`
   completed and moved it to `review`), the precondition at `ent_repository_backlog.go:1215`
   silently returns `applied=false`. This is the same pattern prior research §4.1 already
   flagged generically; concretely here it just means the escalation row is deferred to the next
   tick, not lost — acceptable per the 60s-cadence NFR already accepted project-wide.

## 5. Notification path: how the new signals differ from the existing "use Reset" one

Existing pattern (all four call sites — `session/backlog_lifecycle_review.go:206-211`,
`:658-663`; `session/backlog_lifecycle_pr.go:879-884`, `:1208-1213`;
`session/backlog_lifecycle_stale.go:238-243`; `session/backlog_lifecycle_triage.go:373-378`):
a **one-shot** `l.notify(itemID, title, msg, NOTIFICATION_TYPE_WARNING /*8*/,
NOTIFICATION_PRIORITY_HIGH /*3*/)` fired exactly once at `justParked`, with no durable row of
its own — the *notification event* is ephemeral (goes through `EventBusNotifier`, in-memory
`pkg/events.Bus`, prior research §6/Key Files), even though the underlying `BacklogStuckState`
row for the *original* reason (e.g. `bouncing`) is durable.

Recommended differentiation for the new escalation signal, using headroom that's already
defined in the proto but unused by this notify path:
- **Type/priority**: use `NOTIFICATION_TYPE_ERROR` (7) instead of `WARNING` (8), and
  `NOTIFICATION_PRIORITY_URGENT` (4) instead of `HIGH` (3) — `proto/session/v1/types.proto:794,815`
  defines both and nothing in the backlog lifecycle code currently uses them, so this alone
  makes the toast visually distinct without any new proto work.
- **Durability**: unlike the existing pattern, back the notification with a real
  `BacklogStuckState` row (per §2/§3 above) so the signal survives a missed toast — this is the
  actual gap named in requirements.md's success metrics, not the toast styling.
- **Message content**: for the bounce-cap-exhausted case, the message should say something
  qualitatively different from "use Reset to try again automatically" — per requirements.md
  item 2, capping out *while still bouncing* is evidence the retry loop itself isn't converging,
  not a transient failure, so the copy should say that explicitly (this is a copy/UX decision
  for planning, not an architecture one).

## 6. Data flow: proto/RPC/UI touchpoints

**If both signals are modeled as new `domain.StuckReason` values (recommended, §1-§3):**

- `session/domain/backlog.go` — add `StuckReasonMultipleReasons` and
  `StuckReasonBounceCapExhausted` (or similar names — bikeshed in planning) to the const block
  (lines 41-150) and `AllStuckReasons` (line 153).
- `server/services/backlog_service_stuck.go:28-100` — `toProtoStuckReason`/`fromProtoStuckReason`
  need new `case` arms mapping to new `sessionv1.StuckReason` enum values.
- `proto/session/v1/types.proto` (wherever `StuckReason` enum values like
  `STUCK_REASON_BOUNCING` are defined — not read directly in this pass, but confirmed to exist
  from the `sessionv1.StuckReason_STUCK_REASON_*` constants referenced in
  `backlog_service_stuck.go`) — add two new enum values, run `make proto-gen`.
- `session/backlog_lifecycle.go` — register a new `runStuckDetector("multi_reason_escalation",
  ...)` call, positioned after all reason-specific detectors (§4.1) and likely alongside/before
  `self_heal` (line 1061).
- `session/backlog_lifecycle_review.go:194-223` (`autoReopenWithBackoffGate`) — extend the
  `justParked` branch to also `MarkStuck` the new `StuckReasonBounceCapExhausted` reason.
- `session/backlog_lifecycle.go:1643` (`selfHealStuck`'s switch) — decide/add cases per §4.2.
- **No changes needed** to `ListStuckBacklogItems`, `SnoozeStuckItem`,
  `ResetStuckRemediation`/`BulkResetStuckRemediation`, or any RPC message shape — new reasons
  flow through the existing `StuckBacklogItem` proto message and `FindOpenStuckStates` query
  automatically once the enum mapping exists.
- **Frontend**: `web-app/src/components/backlog-stuck/stuckReason.ts` (label/icon/severity
  lookup keyed by reason string, per its sibling `.test.ts`) needs new entries for the two
  reasons — this is the one place severity styling actually needs to be taught about "these two
  reasons are more severe," rather than inventing a numeric severity field anywhere in the
  backend. `StuckItem.tsx`/`StuckItemDetail.tsx` likely need no structural change if they
  already render arbitrary reasons generically (not verified in this pass — check during
  planning) but may want a visual escalation badge/highlight keyed off the new reason(s) or off
  a computed "this item has N reasons" badge (which itself can be computed client-side in
  `useStuckBacklogItems.ts` by grouping `ListStuckBacklogItems`' flat item list by `itemId` —
  another place the count doesn't require a backend field at all).
- `docs/registry/features/backend/*.json` and `frontend/*.json` — no NEW RPC is being added, so
  the registry-rule trigger is narrower than usual; confirm during planning whether "new proto
  enum values on an existing RPC" requires a registry update per `.claude/rules/feature-registry.md`
  (likely a `lastModified` bump on the existing `list-stuck.json`/`backlog-stuck-items.json`
  entries rather than a new file).

## 7. Event-Command-Policy table (EventStorming grammar)

| Domain Event | Policy trigger | Command | Actor/System |
|---|---|---|---|
| `StuckReasonMarked` (any existing detector's `MarkStuck` succeeds this tick) | new: `reconcileMultiReasonEscalation`, run after all reason detectors (§4.1) | `FindOpenStuckStates` (already-fetched, grouped by item) → count open reasons per item | Reconciler (BacklogLifecycleListener) |
| `OpenReasonCountCrossedThreshold` (count ≥ N, N from planning's Open Question) | `reconcileMultiReasonEscalation` | **new**: `MarkStuck(item, "multiple_reasons", item.Status, ctx)` + notify (ERROR/URGENT) | Reconciler |
| `OpenReasonCountDroppedBelowThreshold` | same detector, next tick | **new**: `ResolveStuck(item, "multiple_reasons")` | Reconciler |
| `RemediationAttemptCapReached` (`justParked=true` from `RemediationDue`) | `autoReopenWithBackoffGate` (`backlog_lifecycle_review.go:194`) — existing, only the bouncing reason's call site matters here | existing: `l.notify(...)` (ephemeral) — **extend to also**: `MarkStuck(item, "bounce_cap_exhausted", BacklogStatusInProgress, ctx)` | Reconciler |
| `BouncingReasonResolved` (PR merged / item shipped / operator reset — existing `resolveStuckLogged(..., StuckReasonBouncing, ...)` call sites, e.g. `backlog_lifecycle.go:1456,1479`) | existing transition call sites | **new**: also `ResolveStuck(item, "bounce_cap_exhausted")` alongside the existing `bouncing` resolve, since the escalation is meaningless once its parent reason clears | Reconciler |
| `StuckItemListRequested` | User opens existing stuck-items UI view | `ListStuckBacklogItems` (unchanged RPC) — now also returns the two new reasons | Human via ConnectRPC/UI |
| `ItemDetailViewed` (item has 2+ open reasons) | Frontend, client-side | **new** (frontend-only): group `ListStuckBacklogItems` result by `itemId`, render count badge — no backend command | Human via UI |

## Key files for implementation planning

- `session/domain/backlog.go` — `StuckReason` const block (41-150), `AllStuckReasons` (153):
  add two new reason constants.
- `session/backlog_lifecycle.go` — `ReconcileStuck` (884), detector registration list
  (947-1079), `runStuckDetector` (869), `selfHealStuck` (1628), `backfillMarkAndNotify` (848).
- `session/backlog_lifecycle_review.go` — `autoReopenWithBackoffGate` (194), `justParked` branch
  (205-212) — the exact call site for signal 2.
- `session/backlog_remediation.go` — `RemediationDue` (168), `MaxRemediationAttempts` (45, = 5,
  not 3 — supersedes prior research's era), `evaluateRemediation` (96).
- `session/storage_backlog.go` — `FindOpenStuckStates` (917), `OpenStuckStateData` (894) — the
  query-time aggregation source for signal 1, no new query needed, just a group-by in the caller.
- `session/ent_repository_backlog.go` — `MarkStuck` (1199, precondition at 1215),
  `ResolveStuck` (1270), `MarkStuckNotified` (1294), existing `RemediationAttemptsGTE` parked
  predicate (1445) confirming the cap-hit condition is already queryable.
- `server/services/backlog_service_stuck.go` — `toProtoStuckReason`/`fromProtoStuckReason`
  (28, 67) need new cases; `ListStuckBacklogItems` (130) needs no change.
- `proto/session/v1/types.proto` — `NotificationType`/`NotificationPriority` enums (781-815,
  `ERROR`=7, `URGENT`=4 both currently unused by this notification path) for signal
  differentiation; `StuckReason` enum (referenced from `backlog_service_stuck.go`, not directly
  read this pass) needs two new values.
- `web-app/src/components/backlog-stuck/stuckReason.ts` — reason→label/severity lookup, needs
  two new entries; `web-app/src/lib/hooks/useStuckBacklogItems.ts` — natural place for a
  client-side per-item open-reason-count grouping if the count badge doesn't need a backend field.
- Prior research doc: `project_plans/backlog-stuck-item-visibility/research/architecture.md` —
  race-condition guidance (§4), notify-once rationale, and the ADR this project's schema
  implements (`project_plans/backlog-stuck-item-visibility/decisions/ADR-001-durable-stuck-state-storage-model.md`,
  not re-read in this pass but referenced by the schema file's own doc comment).
