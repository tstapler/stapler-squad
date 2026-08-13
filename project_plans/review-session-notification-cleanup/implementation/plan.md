# Implementation Plan: Notifications for Headless Review/Triage Sessions

**Feature**: Stop generic TASK_COMPLETE/Idle/Stale notifications from being generated for
ephemeral, `Hidden` review/re-review sessions; stamp `item_id` into notification metadata for
every backlog-linked session so the frontend links "View in Backlog" instead of a dead "View
Session" link; prune notifications whose referenced session/instance no longer exists.
**Date**: 2026-07-25
**Status**: Ready for implementation
**ADRs**: `decisions/ADR-001-notification-record-session-scoped-field.md`

---

## Step 0.5 — Alternatives Considered

**A. Extend the three existing mechanisms in place (CHOSEN).** Add a `Hidden` early-return to
`DefaultStatusDeterminer.Determine`, a `Hidden`-gated suppression + `ItemSession` linkage lookup
to `ReactiveQueueManager.OnItemAdded`, and a predicate-driven `PruneOrphaned` bolted onto
`NotificationHistoryStore.enforceRetention`. *Strength*: zero new dependencies, reuses proven
call sites and test harnesses (`review_queue_manager_test.go`'s `newReactiveQueueTestSetupWithStorage`
already exists for exactly this shape), minimal diff surface. *Weakness*: touches five files
across two packages and requires careful sequencing (Determine's fix must not be conflated with
OnItemAdded's, or the SessionID-overload trap in AC3 gets missed) — mitigated by making each a
separate, independently-testable task below.

**B. Centralize a `NotificationPolicy` interface consulted by every producer before publish.**
*Strength*: one auditable choke point for all future suppression rules. *Weakness*: textbook
interface pollution per `.claude/rules/interface-pollution-checklist.md` — a speculative
interface with a single real call-shape need today, forcing a new dependency through ~9 unrelated
`NewNotificationEvent` call sites for a distinction only 2 of them need. **Rejected.**

**C. Post-hoc filtering at the EventBus or frontend instead of at the publish call sites.**
*Strength*: single interception point, no need to touch multiple backend producers.
*Weakness*: directly contradicts the requirement's explicit constraint ("must be
defense-in-depth at the notification-publishing source(s), not solely a second reliance on the
poller's `shouldSkipSession`"); the EventBus (`pkg/events/bus.go`) has no synchronous
per-event interceptor today (would itself be a new mechanism); and `NotificationHistoryStore`
would still accumulate dead entries needing AC3 regardless, so this doesn't even reduce the
total diff. **Rejected.**

---

## Domain Glossary

| Term | Definition |
|---|---|
| `Instance.Hidden` | Existing `bool` field (`session/instance.go:220-223`) excluding a session from the default session list and review queue. Set `true` only by `SpawnReviewSession` and `TriggerReReview`'s non-headless fallback — the only two call sites in the repo. |
| `ItemSession.SessionRole` | Existing ent string field (`session/ent/schema/item_session.go`), one of `work`/`triage`/`review`. Reached only via the loose FK `session_uuid`, not an ent edge. |
| `ItemSessionSummary` | Existing domain DTO (`session/repository.go:285-308`) returned by `GetItemSessionBySessionUUID`; carries `Role` and `BacklogItemID`. |
| `GetItemSessionBySessionUUID` | Existing method (`session/storage_backlog.go:185-198`, exposed on `*Storage`) — the single-query lookup this plan reuses for both `item_id` enrichment (AC2) and Hidden corroboration. |
| `DetectionResult` / `DetectionAction` | Existing pure-function output type (`session/review_queue_determiner.go:22-31`) describing what the poller should do next (`Skip`/`Add`/`Remove`). |
| `StatusDeterminer.Determine` | Existing pure-function method (`session/review_queue_determiner.go:97-176` and following), evaluated by both `ReviewQueuePoller.checkSession` and `StartupScanner.Scan`. Modified in this plan to short-circuit on `Hidden`. |
| `ReactiveQueueManager.OnItemAdded` | Existing observer callback (`server/review_queue_manager.go:319-373`), the sole place a `ReviewItem` becomes a persisted+broadcast notification. Modified in this plan to resolve backlog linkage and suppress for `Hidden` sessions. |
| `hiddenSession` | New local `bool` in `OnItemAdded`, `true` iff the resolved `*session.Instance` is non-nil and `Hidden`. |
| `itemSessionLookupTimeout` | New `const` (`server/review_queue_manager.go`), a short bounded timeout (2s) for the synchronous `GetItemSessionBySessionUUID` call added to `OnItemAdded`'s observer path. |
| `NotificationRecord` | Existing flat-JSON-persisted struct (`server/notifications/store.go:34-52`). Gains a new `SessionScoped bool` field (ADR-001). |
| `NotificationRecord.SessionScoped` | New field, `true` only when the record's producer explicitly marked `metadata["session_scoped"] = "true"` — the positive signal that `SessionID` is a real session identifier, not an overloaded backlog-item ID. See ADR-001. |
| `NotificationHistoryStore.PruneOrphaned` | New exported method — removes `SessionScoped` records with no `item_id` whose `SessionID` is absent from a batch-fetched in-memory set of existing session IDs. Returns the count removed. |
| `NotificationHistoryStore.SetSessionExistenceLookup` | New exported setter (renamed from an earlier per-record-predicate design after architecture review flagged the N+1 cost) — wires a **batch** `existingSessionIDs func() map[string]struct{}` function, called ONCE per prune pass (not once per eligible record) to fetch the full existing-ID set, which all eligible records are then checked against via map membership. `nil` (the default, or the closure itself returning `nil` for a given call) makes that pass a no-op — see `pruneOrphanedMinUptime` below for the one case the closure intentionally returns `nil`. |
| `NotificationHistoryStore.lastOrphanPruneAt` / `orphanPruneInterval` | New unexported field + `const` (`server/notifications/store.go`, default 1 minute) — decouples the orphan sweep (a real batch fetch) from firing on literally every `Append()` call (roughly every 500ms under the subscriber's coalesce interval). `enforceRetention()` only invokes the orphan sweep when `time.Since(lastOrphanPruneAt) >= orphanPruneInterval`; the pre-existing age/count trim in the same function stays unconditional because it is pure in-memory slice work with no I/O cost. |
| `events.SessionScopedMetadata` | New helper function (`pkg/events`, forwarded through `server/events` alongside `NewNotificationEvent`) — `func(base map[string]string, itemID string) map[string]string`. Builds and returns a **fresh** map (copying `base`'s entries, never mutating `base`) with `MetadataKeySessionScoped` set to `"true"` and `MetadataKeyItemID` set to `itemID` when non-empty. The single shared call site for the `"session_scoped"`/`"item_id"` metadata convention (Tasks 2.2.1a, 3.2.1b); `MetadataKeySessionScoped`/`MetadataKeyItemID` are the paired exported constants `eventToRecord` (Task 4.1.1b) reads back. |
| `eventToRecord` | Existing conversion function (`server/notifications/subscriber.go:135-164`) — the sole place an `*events.Event` becomes a `*NotificationRecord` before `Append`. Modified to populate `SessionScoped`. |
| `metadata["session_scoped"]` (`events.MetadataKeySessionScoped`) | New convention key on the `map[string]string` passed to `events.NewNotificationEvent`, set only by the two producers whose `SessionID` is a real session identifier — both via `events.SessionScopedMetadata`, never as an independent string literal per-producer. |
| `metadata["item_id"]` | Existing convention key (already used throughout `backlog_service_triage.go`) identifying the backlog item a notification is about; the frontend already prefers this for "View in Backlog" routing (`NotificationsPage.tsx:377-393`, no change needed). |
| `StartupScanner.Scan` | Existing method (`session/startup_scanner.go:31-53`) that runs `Determine()` on every loaded instance before the first poll tick. Gains a cheap `Hidden` pre-check (belt-and-suspenders; the `Determine()` fix alone already makes this behaviorally redundant, but it's free and closes the specific reproducible bypass at its own source). |
| `onAutonomousDriverComplete` | Existing method (`server/services/autonomous_orchestration_service.go:228-546`) — a second, independent generic-completion notifier for `AutonomousDriver`-run sessions. Two call sites inside it are fixed: the "Triage stuck" notice (~line 310, metadata only) and the generic done/stuck notice (~line 540, metadata + `Hidden` gate). |
| `linkedItemID` | New outer-scope local `string` in `onAutonomousDriverComplete`, threading the resolved backlog item ID from the nested `ItemSession`/`BacklogItem` lookup block out to the generic notifier call at the bottom of the function. |
| `pruneOrphanedMinUptime` | New `const` (`server/server.go`, 5 minutes) — a defensive minimum-process-uptime gate: while `time.Since(srv.startedAt) < pruneOrphanedMinUptime`, the wired existence-lookup closure returns `nil` (meaning "not ready to judge, skip this pass") instead of the real batch-fetched set, guarding against any future regression that makes instance loading asynchronous (see Risk Control). |
| `Server.startedAt` | New `time.Time` field on the `Server` struct (`server/server.go`), set once in `newServerBase` — the single shared construction path `NewServer` and `NewServerWithDeps` both call before either computes anything server-specific. Replaces a `NewServer`-local `startTime` variable that `wireDepsIntoServer` (where the existence-lookup closure is actually built) could never have seen, since `NewServerWithDeps` calls `wireDepsIntoServer` directly without ever going through `NewServer` at all. |
| `AttentionReason` | Existing type alias (`session/review_queue.go:12`) for `queue.AttentionReason`; `ReasonTaskComplete`/`ReasonIdle`/`ReasonStale` are the three reasons this feature suppresses for `Hidden` sessions — and, per the narrowed Design Decision 1, the *only* reasons suppressed: `ReasonErrorState`/`ReasonTestsFailing` still notify even when `Hidden`, as a safety net for an alive-but-stuck review session with no other durable detector watching it (see Design Decision 1 below). |
| `reconcileStuckReviewItems` / `markAbandonedReview` / `StuckReasonAbandonedReview` | Existing durable stuck-detection machinery (`session/backlog_lifecycle.go:1568-1642`, `:1772-1828`), run periodically by `BacklogLifecycleListener.ReconcileStuck`'s sweep — entirely independent of the review-queue/`OnItemAdded` notification path this plan modifies. Detects a review-status backlog item with no active (`EndedAt`-nil) review/work session (`FindStuckReviewItems`) or a confirmed-dead zombie session (`FindZombieReviewItems`), and durably marks + notifies (once) + can auto-respawn review. Covers a `Hidden` review session that **crashes or exits** before calling `submit_review_verdict`; does **not** cover one that is still *alive* but stuck (see Design Decision 1's narrowing). |
| `wireRateLimitCallbacks` / `onDetected` / `onRecovery` | Existing method (`server/services/session_service.go:3848-3898`) and its two closures, registered on every `Instance` via `wireCallbacks` — including `Hidden=true` review sessions, since `CreateDirectorySession`/`CreateWorktreeSession`/`WireInstanceCallbacks` have no `Hidden` branch. A third unguarded `events.NewNotificationEvent` producer identified during the Phase 4 pre-mortem (Failure #1, Finding 1) and closed by this plan's new Epic 5. |
| `rateLimitLookupTimeout` | New `const` (`server/services/session_service.go`, 2s, Epic 5) — bounds the `GetItemSessionBySessionUUID` lookup added inside `wireRateLimitCallbacks`'s closures, which (unlike `OnItemAdded`) receive no `ctx` parameter from their caller, so `context.WithTimeout(context.Background(), ...)` is used directly rather than a `baseContext()`-style helper. |
| `concStorage` | Existing field (`server/services/session_service.go:82`, `*session.Storage`) — "the concrete backing store...used for operations...not part of the `InstanceStore` interface...nil when storage is a fake `InstanceStore` (tests)". Reused by Epic 5's `wireRateLimitCallbacks` fix to call `GetItemSessionBySessionUUID`, which `s.storage`'s `session.InstanceStore` interface type does not expose — the same "lookup already cheaply available at this call site" pattern `rqm.storage` (a `*session.Storage` field, not an interface) already uses in `server/review_queue_manager.go`. |

---

## Pattern Decisions

| Decision Point | Pattern Applied | Alternative Rejected | Reason |
|---|---|---|---|
| Where to gate on `Hidden` | Pure-function early-return (`Determine`) + defense-in-depth guard at the publish call site (`OnItemAdded`) — two independent checks, not one relied on twice | Single check only in `shouldSkipSession` (poller) | Requirements explicitly forbid relying solely on the poller's existing `shouldSkipSession`; `StartupScanner.Scan` proven to bypass it entirely (architecture.md Q1) |
| Resolving `item_id`/`SessionRole` | Reuse existing single-query DTO (`GetItemSessionBySessionUUID` → `ItemSessionSummary`) | New dedicated query/dedicated repository method | Interface-pollution checklist: don't add a second query for data one call already returns; `maybeAutoCreatePR` in the same file already calls this exact method for the same session |
| Session-scoped vs. item-scoped notification discrimination (AC3) | New explicit typed field (`NotificationRecord.SessionScoped`), see ADR-001 | Infer from `review-queue-<sessionID>-<timestamp>` ID-prefix string matching | Prefix-matching is an inferred, unaudited signal that silently breaks if any future producer picks a colliding ID scheme; an explicit field makes the eligibility decision visible and mandatory-opt-in at the producer |
| Orphan existence check | Inject a batch `func() map[string]struct{}` (`existingSessionIDs`) into `NotificationHistoryStore`, called ONCE per prune pass and wired from `server.go` around `session.Storage.ListInstanceData()` | (a) A plain per-record `func(sessionID string) bool` predicate; (b) Import `*session.Storage`/`*session.ReviewQueuePoller` directly into the `notifications` package | (a) rejected by architecture + adversarial review: a per-record predicate backed by `FindInstanceDataByID` re-runs a full multi-edge-eager-loaded ent query (`ListInstanceData` → `EntRepository.List`) once per eligible record, up to 500 times per prune pass, while `s.mu` is held — an O(eligible-records) DB-query storm on a hot path; (b) still rejected per the interface-pollution checklist (define at consumption point) + pitfalls.md's package-layering note: `server/notifications` currently has zero dependency on `session` and should stay that way |
| Existence check data source | `session.Storage.ListInstanceData()` (durable, synchronously loaded), fetched once per prune pass into an in-memory `map[string]struct{}` keyed by both stable ID and title | `session.ReviewQueuePoller.FindInstance` (live in-memory only) | `FindInstance` only reflects the poller's currently-monitored set — absent after a restart before reconciliation even though the session is real; `ListInstanceData` reflects the durable record set loaded synchronously in `BuildRuntimeDeps` before the async tmux-start goroutine even begins (see Design Decision 2) |
| Orphan sweep cadence | Gated inside `enforceRetention()` by `time.Since(lastOrphanPruneAt) >= orphanPruneInterval` (1 min default), so the batch fetch runs on a coarse timer, not on every `Append()` | Run the batch fetch unconditionally on every `Append()` (the plan's original shape) | `Append()` fires roughly every 500ms under the subscriber's coalesce interval; even a single once-per-pass batch fetch is a real ent query and should not run twice a second while `s.mu` (a write lock) is held, blocking all concurrent notification reads/writes |
| Suppression scope once `Hidden==true` | Scoped to only `Reason ∈ {ReasonTaskComplete, ReasonIdle, ReasonStale}` (AC1's literal three reasons), leaving `ReasonErrorState`/`ReasonTestsFailing` notifying even for `Hidden` sessions | Unconditional on `Reason` (the plan's original choice, matching the pre-existing `shouldSkipSession` invariant "any reason") | Verified (Phase 4 pre-mortem Failure #3) that this repo's durable stuck-detection sweep (`reconcileStuckReviewItems`/`markAbandonedReview`, `session/backlog_lifecycle.go`) only fires once a review session has *ended* or is a confirmed zombie — it does not watch a still-alive session stuck in an error state. Leaving `ReasonErrorState`/`ReasonTestsFailing` notifying for `Hidden` sessions is the only signal covering that residual gap (see Design Decision 1) |
| Suppression trigger boolean | `Hidden` as the sole necessary-and-sufficient gate at both `Determine()` and `OnItemAdded`; `SessionRole` used only as enrichment/corroboration, never an independent OR | Suppress independently on `SessionRole == review \|\| SessionRole == triage`, per the requirements' literal "OR" wording | pitfalls.md: `SessionRoleReview` is also used for a session that becomes the review session on reopen in *non-Hidden* flows (`backlog_lifecycle.go:3141` pairs `SessionRoleWork`/`SessionRoleReview`); an independent Role-only OR risks silently swallowing a real, visible session's notifications (see Design Decision 1) |

---

## Migration Plan

**Omitted — confirmed no schema change.** `NotificationRecord.SessionScoped` (ADR-001) is a new
field on a plain Go struct serialized to a flat JSON file
(`~/.stapler-squad/.../notifications.json`) via `server/notifications/store.go`'s
`loadFromDisk`/`saveToDisk` — entirely separate from `session/ent/schema/*`. Old records decode
with `SessionScoped` defaulting to its zero value (`false`, the safe/never-pruned default) with
no migration script or `--feature sql/upsert` regeneration required. No ent schema field is
touched anywhere in this plan.

---

## Design Decisions (resolving the explicitly-flagged open questions)

### 1. `Hidden` vs. `SessionRole` — the exact boolean

**Resolution**: `Hidden` is the sole necessary-and-sufficient suppression *trigger* — `SessionRole`
is never checked as an independent OR branch anywhere in this plan — but the suppression it gates
is **narrowed to `Reason ∈ {ReasonTaskComplete, ReasonIdle, ReasonStale}`**, not unconditional on
`Reason`. A `Hidden` session that reaches `ReasonErrorState`, `ReasonTestsFailing`,
`ReasonApprovalPending`, or `ReasonInputRequired` still notifies, applied identically at both
`Determine()` and `OnItemAdded`.

*(Revised during the Phase 4 pre-mortem's P1 Failure #3: the plan's original "unconditional on
Reason" choice was verified against this repo's actual durable stuck-detection machinery — see the
narrowing rationale below — and found to leave a real gap for a specific sub-case, not a
hypothetical one.)*

**`Hidden` vs. `SessionRole` as the trigger** — unchanged from the original resolution and still
correct:

- The only two call sites that ever set `Hidden = true`
  (`server/services/session_service.go:827` `SpawnReviewSession`, and
  `server/services/backlog_service_triage.go:2351-2352` `TriggerReReview`'s non-headless
  fallback) **both also** create their `ItemSession` row with `SessionRole: SessionRoleReview`
  (`session/review_gate.go:346-352`). The two signals always agree for every real call site that
  exists today — there is no case in the current codebase where `Hidden==true` and
  `SessionRole != review`, or vice versa for a session that should be suppressed.
- Headless-triage sessions (`TriggerTriage`, `session/backlog_service_triage.go:1781-1798`) never
  construct a `session.Instance` at all (features.md) — they are structurally invisible to
  `Determine()`/`OnItemAdded` regardless of which boolean gates suppression, so `SessionRole ==
  triage` never needs to be an independent trigger in this codepath; nothing reaches it to
  suppress.
- Checking `SessionRole` independently would be *unsafe*, not merely redundant:
  `SessionRoleReview` is also used for the session a *non-hidden* work session becomes on reopen
  in several flows (`session/backlog_lifecycle.go:3141` explicitly pairs `SessionRoleWork` and
  `SessionRoleReview`; also `backlog_service.go:732,773,810`). An OR-on-Role-alone check risks
  silently swallowing a real, visible session's legitimate notifications the first time such a
  flow is exercised.

`SessionRole` is still fetched (via `GetItemSessionBySessionUUID`, already required for AC2's
`item_id` enrichment) and used for corroboration in tests and logging, but never as a second,
independent suppression path.

**Cross-reference (Finding 4, plan-repair pass)**: `requirements.md`'s Acceptance Criterion 1 was
amended to state this resolved `Hidden`-as-sole-trigger boolean precisely (it previously read as a
literal `Hidden == true OR SessionRole ∈ {review, triage}`, which this Design Decision already
correctly did not implement) — the two documents now agree; see AC1 there for the exact wording.

**Why "unconditional on Reason" was narrowed** — verified against this repo's actual stuck-review
machinery, not assumed: `session/backlog_lifecycle.go`'s periodic `ReconcileStuck` sweep already
runs a `reconcileStuckReviewItems` detector (`session/backlog_lifecycle.go:1568-1642`) wired to two
durable, DB-backed queries — `EntRepository.FindStuckReviewItems`
(`session/storage_backlog.go:700-715`, items in "review" status with no active, i.e.
`EndedAt`-nil, review/work session) and `EntRepository.FindZombieReviewItems`
(`session/storage_backlog.go:1043`, an `EndedAt`-nil session whose underlying process is confirmed
dead via a liveness checker) — both feeding `markAbandonedReview`
(`session/backlog_lifecycle.go:1772-1828`), which writes a durable `StuckReasonAbandonedReview`
row, sends a notify-once operator notification (`l.notify(itemID, "Review item needs attention",
...)`, already `item_id`-tagged, never subject to this plan's `PruneOrphaned`/`SessionScoped`
machinery), and can auto-respawn the review. **This mechanism already covers a `Hidden` review
session that crashes or exits before calling `submit_review_verdict`** — the moment its
`ItemSession` row's `EndedAt` is set (normal exit) or its process is confirmed dead (zombie), the
next `ReconcileStuck` tick picks it up independent of the review-queue/`OnItemAdded` path this plan
touches.

**What it does *not* cover**: a `Hidden` review session that is still *alive* — tmux process still
running, `ItemSession.EndedAt` still nil — but stuck in `ReasonErrorState`/`ReasonTestsFailing`
(detected by `Determine()`, `session/review_queue_determiner.go:138-147`, while
`statusInfo.IsControllerActive` is true or content-detection finds the same pattern) and never
progressing to a verdict. Both `FindStuckReviewItems` and `FindZombieReviewItems` require the
session to have already ended or be confirmably dead — neither fires for a session that is simply
alive-but-erroring. For this specific sub-case, the review-queue's own error/tests-failing
detection was the *only* signal watching an alive session's health; suppressing it unconditionally
for `Hidden` sessions (the plan's original choice) would have removed that signal with nothing else
picking up the slack. Narrowing suppression to only `ReasonTaskComplete`/`ReasonIdle`/`ReasonStale`
(AC1's literal three reasons) leaves `ReasonErrorState`/`ReasonTestsFailing` notifying for `Hidden`
sessions specifically to close this residual gap, while `ReasonApprovalPending`/`ReasonInputRequired`
were never suppressed to begin with (pre-existing `OnItemAdded` guard, unrelated to this plan).

### 2. AC3's existence check function + post-restart reload window

**Resolution**: `session.Storage.ListInstanceData()` (`session/storage.go:381`), fetched **once
per prune pass** and filtered in memory — **not** a per-record call to `FindInstanceDataByID`,
and **not** `ReviewQueuePoller.FindInstance` alone.

*(Revised after architecture + adversarial review both independently flagged the original
per-record-predicate shape as an N+1-under-lock: `FindInstanceDataByID` itself calls
`ListInstanceData()` → `EntRepository.List(ctx)`, a full ent query eager-loading
`Worktree`/`Tags`/`Project`/`ClaudeSession+Metadata` for every stored session — not a cheap
lookup — so calling it once per eligible `SessionScoped` record inside `pruneOrphanedRecords`'s
loop, on every single `Append()`, is a DB-query storm on a hot path while the store's write lock
is held. The fix keeps the same underlying data source but calls it once and checks membership.)*

`ListInstanceData()` reads the durable, disk/ent-backed instance record list, which is populated
**synchronously** in `BuildRuntimeDeps` (`server/dependencies.go:462`, `instances, err :=
storage.LoadInstances()`) **before** the async background goroutine that starts tmux processes
and reconciles Stopped sessions even begins (`server/dependencies.go:~592` onward). The specific
race pitfalls.md warns about — `DoesSessionExist()`/`recoverFromServerFailure` treating "not yet
visible in some in-memory collection at this exact moment" as "gone forever" — is a *different*
mechanism (the live actor/tmux registry), not the durable storage list this plan checks. Reading
`ListInstanceData()` therefore does not depend on the async reload step completing at all.

The wired existence-lookup closure (`NotificationHistoryStore.SetSessionExistenceLookup`, a
`func() map[string]struct{}`) fetches `storage.ListInstanceData()` exactly once each time it is
invoked (once per gated prune pass — see the new `orphanPruneInterval` cadence in Story 4.2),
builds a `map[string]struct{}` keyed by each instance's stable ID and title (mirroring
`InstanceData.MatchesID`'s own two-way match), and returns that set. As defensive
belt-and-suspenders against any *future* change that makes storage loading asynchronous too, the
closure additionally returns `nil` (a distinct sentinel from "the real set, which happens to be
empty," meaning "not ready to judge existence this pass, prune nothing") for the first
`pruneOrphanedMinUptime` (5 minutes) of process uptime, measured from the new `srv.startedAt`
field (see Task 4.3.1a for why this replaced a `NewServer`-local `startTime`).

### 3. AC3's `SessionID`-overload discriminator

**Resolution**: new explicit `NotificationRecord.SessionScoped bool` field, set only by the two
producers whose `SessionID` is a genuine session identifier. Full justification in
`decisions/ADR-001-notification-record-session-scoped-field.md`. `PruneOrphaned`'s eligibility
check: `record.SessionScoped && record.Metadata["item_id"] == "" && !existingIDs[record.SessionID]`,
where `existingIDs` is the single batch-fetched `map[string]struct{}` for the current prune pass
(see Design Decision 2), not a per-record function call.

### 4. Exact insertion point for `Determine()`'s `Hidden` check

**Resolution (revised — see Design Decision 1's narrowing)**: because suppression is now
*reason-scoped*, not unconditional, the check can no longer be a first-statement early-return —
`reason` isn't known until the existing detection logic has run. Instead, gate the final
`action` computation at the bottom of `func (d *DefaultStatusDeterminer) Determine(...)
DetectionResult` (`session/review_queue_determiner.go:288-300`), immediately before the
function's single terminal `return`:

```go
	action := DetectionActionSkip
	if shouldAdd {
		action = DetectionActionAdd
	}

	// Hidden (system/background) sessions never surface a generic completion/idle/stale
	// notification — mirrors ReviewQueuePoller.shouldSkipSession's existing invariant
	// (review_queue_poller.go:629), but applied here too so StartupScanner.Scan (which calls
	// Determine() directly without going through shouldSkipSession) cannot bypass it. Narrowed
	// to exactly these three reasons (not unconditional on reason, see Design Decision 1):
	// ReasonErrorState/ReasonTestsFailing still notify even when Hidden, because this repo's
	// durable stuck-review sweep (reconcileStuckReviewItems, session/backlog_lifecycle.go) only
	// detects a review session once it has ended or is a confirmed zombie — it does not watch
	// a still-alive session stuck in an error state, and this is the only signal that does.
	if inst.Hidden && action == DetectionActionAdd &&
		(reason == ReasonTaskComplete || reason == ReasonIdle || reason == ReasonStale) {
		action = DetectionActionSkip
	}

	return DetectionResult{
		Action:        action,
		Reason:        reason,
		Priority:      priority,
		Context:       ctx,
		ClaudeStatus:  claudeStatus,
		CleanWorktree: cleanWorktree,
	}
}
```

The two existing early `return DetectionResult{Action: DetectionActionRemove, ...}` statements
inside the `IsControllerActive`/no-controller branches (`IdleStateActive` at line ~163;
`StatusExecuting`/`StatusProcessing`/`StatusWaitingForAgent` at line ~231) are untouched — they
already return `Remove`, never `Add`, so they can never produce a notification and need no Hidden
gate. Unlike the plan's original first-statement early-return, this shape means `Determine()` runs
its full detection logic even for `Hidden` instances (a `Hidden` instance no longer short-circuits
before `claudeStatus := statusInfo.ClaudeStatus`) — a deliberate, small extra amount of in-memory
CPU work per poll tick (no new I/O), traded for the ability to still classify and act on
`ReasonErrorState`/`ReasonTestsFailing` for a `Hidden` instance.

### 5. `StartupScanner.Scan`'s exact fix

**Resolution (revised — no change to `StartupScanner.Scan` after all, see Design Decision 1's
narrowing)**: the plan originally proposed adding `|| inst.Hidden` to
`session/startup_scanner.go:35`'s skip condition as "belt-and-suspenders." That addition is now
**dropped** — it would unconditionally skip `GetStatus`/`GetContent`/`Determine()` for every
`Hidden` instance at startup regardless of reason, silently reintroducing exactly the
over-suppression Design Decision 1's narrowing exists to prevent (a `Hidden` instance sitting in
`ReasonErrorState` at the moment of a server restart would never be evaluated at all, and so would
never notify, even though Design Decision 4's `Determine()` fix says it should). `Determine()`
itself is now the single, sufficient place `Hidden` + `Reason` are both known and the narrowed
gate can be correctly applied — `StartupScanner.Scan` calling `Determine()` unmodified is no
longer a bypass, it's simply delegating to the one function equipped to decide correctly. No code
change to `session/startup_scanner.go` is required by this plan. (Story 1.2 below is retained only
as a regression test proving `StartupScanner.Scan` still calls `Determine()` for a `Hidden`
instance and gets the narrowed result back, not as a source-code change.)

### 6. `autonomous_orchestration_service.go`'s two call sites

**"Triage stuck" notice** (`onAutonomousDriverComplete`, ~line 310): metadata-only fix, **no**
`Hidden` gate added. `item.ID` is already in scope; change the trailing `nil` argument to
`map[string]string{"item_id": item.ID}`. No suppression logic is added here because (a) this
notification's content is a distinct, actionable "driver got stuck" signal, not one of AC1's
three generic reasons, matching the existing `SessionRoleReview` precedent a few lines below of
*not* suppressing a role-specific notification; and (b) per features.md this branch is currently
dead code in practice (no live caller attaches `SessionRoleTriage` to a real
`AutonomousDriver`-run `Instance`) — adding suppression logic to an unreachable path would be
unverifiable and untestable without contriving an artificial harness, whereas the metadata fix
is cheap, always-correct insurance regardless of reachability.

**Generic done/stuck notice** (~line 540): both a metadata fix and a `Hidden` gate, because this
notifier is the functional twin of AC1's generic-completion notification for
`AutonomousDriver`-run sessions ("Autonomous fix complete"/"Autonomous fix stuck" mirror
`ReasonTaskComplete`/`ReasonStale`'s semantics) and `inst` (the real `*session.Instance`,
resolved earlier in the function via `a.instanceFinder(instanceName)`) is already in scope with
zero extra lookup cost. A new outer-scope `linkedItemID string` variable is set inside the
nested `ItemSession`/`BacklogItem` lookup block (where `item.ID` is available) and threaded down
to this call site's call to the shared `events.SessionScopedMetadata(nil, linkedItemID)` helper
(see the Concern fix below) in place of a hand-built map literal. The whole publish is wrapped in
`if !inst.Hidden { ... }`.

**Shared metadata helper (architecture-review Concern fix, applied here and in Epic 2).** Both
this call site and `OnItemAdded` (Task 2.2.1a) build a near-identical
`{"item_id": ..., "session_scoped": "true"}` map; per the design-patterns skill's "generalize once
2+ real call sites need identical logic" guidance, this plan extracts one shared helper —
`events.SessionScopedMetadata(base map[string]string, itemID string) map[string]string`
(`pkg/events`, forwarded through `server/events`) — instead of leaving `"session_scoped"` as an
independent string literal in three files with no shared constant. `base` is only ever read
(copied into a fresh map), never mutated — see Blocker-A fix in Story 2.1/2.2 below for why that
matters here too.

---

## Event-Command-Policy Table

| Domain Event | Policy Trigger | Command | Actor |
|---|---|---|---|
| `HiddenInstanceEvaluated` | `Determine()` reaches its final action computation with `inst.Hidden == true` | `SkipDetection` (force `DetectionActionSkip`) **only when** `reason ∈ {ReasonTaskComplete, ReasonIdle, ReasonStale}`; `ReasonErrorState`/`ReasonTestsFailing` still produce `DetectionActionAdd` | `DefaultStatusDeterminer.Determine` |
| `HiddenInstanceLoadedAtStartup` | `StartupScanner.Scan` iterates an instance with `Hidden == true` | *(no separate command — `StartupScanner.Scan` still calls `Determine()` unmodified, which applies `HiddenInstanceEvaluated`'s narrowed gate itself; see Design Decision 5)* | `StartupScanner.Scan` |
| `ReviewQueueItemAdded` | `queue.Add()` transitions `exists:false → true` for `Reason != ReasonApprovalPending` | `ResolveSessionLinkage` (`FindInstance` + `GetItemSessionBySessionUUID`) | `ReactiveQueueManager.OnItemAdded` |
| `BacklogLinkedSessionResolved` | Linkage lookup returns `BacklogItemID != ""` | `StampItemIDMetadata` | `ReactiveQueueManager.OnItemAdded` |
| `HiddenSessionResolved` | Resolved `*Instance` is non-nil, `Hidden == true`, and `item.Reason ∈ {ReasonTaskComplete, ReasonIdle, ReasonStale}` | `SuppressNotificationPublish` (skip `eventBus.Publish`) — **not** triggered for `ReasonErrorState`/`ReasonTestsFailing`, which still publish even when `Hidden` | `ReactiveQueueManager.OnItemAdded` |
| `AutonomousDriverCompleted` | `onAutonomousDriverComplete` fires for a real, driver-run `Instance` | `StampItemIDMetadata` always; `SuppressIfHidden` on the generic done/stuck notifier only (unconditional on reason here — this notifier only ever encodes done/stuck semantics, never a distinct error-class signal, see Design Decision 6) | `AutonomousOrchestrationService.onAutonomousDriverComplete` |
| `RateLimitCallbackFired` | `inst.SetRateLimitCallbacks`'s `onDetected`/`onRecovery` closures fire for a rate-limit transition | `StampItemIDMetadata` when backlog-linked; `SuppressIfHidden` (unconditional on reason — a rate-limit event has no `AttentionReason`/error-class distinction to preserve, see Epic 5) | `SessionService.wireRateLimitCallbacks` |
| `NotificationAppended` | Every `Append()` call | `EnforceRetention` (existing age/count, unconditional) then, only when `time.Since(lastOrphanPruneAt) >= orphanPruneInterval`, `PruneOrphanedRecords` (new) | `NotificationHistoryStore.Append` |
| `OrphanSweepDue` | Gated prune pass fires (`orphanPruneInterval` elapsed since the last sweep) | `FetchExistingSessionIDs` (batch call to the injected `existingSessionIDs func() map[string]struct{}` → `storage.ListInstanceData()` once, gated by `pruneOrphanedMinUptime`) then filter all eligible records in memory | `NotificationHistoryStore` (via `existenceChecker`) |
| `SessionRecordConfirmedGone` | Filtered record's `SessionID` absent from the fetched set | `DeleteNotificationRecord` | `NotificationHistoryStore.PruneOrphaned` |

---

## Observability Plan

- `Determine()`'s Hidden early-return and `StartupScanner.Scan`'s Hidden skip are silent (no new
  log line) — matches the existing silent behavior of `shouldSkipSession`, which also logs
  nothing on skip; adding logging here would spam every poll tick for any Hidden session that
  stays alive a while.
- `OnItemAdded`'s new `GetItemSessionBySessionUUID` lookup failure (context deadline, ent error
  other than not-found) is logged at `Warn` via `log.Warn(...)`, mirroring the existing
  `maybeAutoCreatePR` pattern (`server/review_queue_manager.go:426-427`) — a real lookup failure
  must not take the same silent path as "not backlog-linked."
- `NotificationHistoryStore.PruneOrphaned` logs at `Info` the count removed when non-zero
  (`log.Info("NotificationHistoryStore: pruned orphaned records", "count", removed)`), giving an
  operator a visible signal the sweep is running and finding real work, without logging on every
  no-op pass.
- No new metrics/traces are introduced — this feature is a correctness fix to existing
  notification plumbing, not a new subsystem; existing EventBus/notification-store observability
  (if any) already covers the modified call sites.

---

## Risk Control

| Risk | Mitigation | Where addressed |
|---|---|---|
| Over-suppression: a real, visible session's notifications silently disappear | `Hidden` (not `SessionRole` alone) is the sole trigger; confirmed both real `Hidden=true` call sites also carry `SessionRole=review`, so no behavior gap; `SessionRole` never checked independently | Design Decision 1 |
| Hot-loop I/O: adding a DB lookup to a 2s-tick concurrent poll loop | `ItemSession` lookup lives only in `OnItemAdded` (fires once per queue *transition*, not per tick) — `Determine()`/`checkSession` gain zero new I/O, `Hidden` is an in-memory field check | Design Decision 1, Task 1.3.1 |
| Post-restart mass-pruning: treating "not yet reloaded" as "gone forever" | `ListInstanceData()` reads the durable, synchronously-loaded storage list (not the async-reconciled live registry) + defensive `pruneOrphanedMinUptime` (5 min) belt-and-suspenders, where the existence-lookup closure returns `nil` (not an empty set) to mean "skip this pass" | Design Decision 2, Task 4.3.1a |
| SessionID overload: pruning deletes legitimate item-scoped notifications | Explicit `SessionScoped` field, opt-in only at the two real session-scoped producers (ADR-001) | Design Decision 3, Epic 3 |
| DB-query storm on a hot path: an existence check per eligible record, on every `Append()`, under the store's write lock | (a) Existence check is a **batch** fetch (`existingSessionIDs func() map[string]struct{}`), called once per prune pass and checked via in-memory map membership, not once per record; (b) the sweep itself is gated to run at most once per `orphanPruneInterval` (1 min default) inside `enforceRetention()`, not on every single `Append()` | Design Decision 2, Task 4.2.1a, Task 4.2.1b |
| Blocking the synchronous `OnItemAdded` observer callback on a slow DB call | Bounded `itemSessionLookupTimeout` (2s) via `context.WithTimeout(rqm.baseContext(), ...)`, same pattern as `maybeAutoCreatePR`'s `autoCreatePRLookupTimeout` | Task 1.3.1 |
| Regression: existing `ReasonApprovalPending`/`ReasonInputRequired`/`ReasonErrorState` tests break | New suppression is additive and reason-scoped (`&& !(hiddenSession && item.Reason is TaskComplete/Idle/Stale)`) alongside the existing `&& item.Reason != session.ReasonApprovalPending` condition; `TestOnItemAdded_EventBusBehavior_BUG001`'s three existing cases use non-Hidden fixture instances (no `poller.SetInstances` call in that test → `FindInstance` returns nil → `hiddenSession` is always `false`) so all three continue to pass unmodified | Task 1.3.3 |
| Dead-code fix (Triage-stuck `Hidden` gating) impossible to verify | Deliberately *not* added (Design Decision 6) — verified-unreachable logic isn't worth an untestable diff; metadata-only fix is cheap and always correct | Design Decision 6 |
| Under-suppression: a `Hidden` review session stuck in a genuine error state (`ReasonErrorState`/`ReasonTestsFailing`) has no other durable signal watching it while still alive | Narrowed Design Decision 1: these two reasons are excluded from the `Hidden` suppression scope, so they still notify; separately, this repo's `reconcileStuckReviewItems`/`markAbandonedReview` sweep (`session/backlog_lifecycle.go`) already covers the *exited-or-zombie* sub-case (a `Hidden` review session that crashes/exits before calling `submit_review_verdict`) via a fully independent, durable, notify-once path | Design Decision 1, Story 1.1, Story 2.2 |
| **Accepted residual gap (Phase 4 triad-review UX finding): a `Hidden` review session that is genuinely still alive but hung/silent (`ReasonIdle`/`ReasonStale`) before calling `submit_review_verdict` has zero durable signal after this fix ships.** Unlike the `ReasonErrorState` case above, `ReasonIdle`/`ReasonStale` are not left un-suppressed — AC1's literal text (and the backlog item's own title, which quotes "Session idle - ready for next task" verbatim as one of the two complained-about strings) explicitly requires suppressing exactly these two reasons for `Hidden` sessions. No `StuckReason` constant (`session/domain/backlog.go`, audited in full: `PRReadyUnmerged`/`ReworkCap`/`AbandonedReview`/`StaleWork`/`Bouncing`/`PushFailed`/`OrphanedTriage`/`AutonomousStuck`/`SpawnFailed`/`PlanNotApproved`/`PRPendingNoPR`/`ReworkBlockedStale`) covers "a still-alive review session, not yet exited, not yet erroring, producing no new output" — `AbandonedReview` requires the session to have already ended, and `ReworkBlockedStale` covers the prior *work* session stalling after a FAIL verdict, not the review session itself before any verdict. **This is a real, accepted trade-off, not an oversight**: before this fix, the only signal for this exact case was the same noisy, dead-linked notification this item exists to eliminate — so shipping this fix does not make a previously-well-monitored case newly invisible, it removes a low-value signal for a case that already had no *actionable* signal (clicking through led nowhere). Recommended as a separate, out-of-scope follow-up backlog item: extend `session/backlog_lifecycle.go`'s stuck-detection sweep with a new reason (e.g. `StuckReasonReviewSessionSilent`) covering "review-status item's linked review `ItemSession` has no `EndedAt` and no recent `LastMeaningfulOutput`/`LastProgressAt`," mirroring `StuckReasonStaleWork`'s existing shape but scoped to `SessionRoleReview` instead of `SessionRoleWork`. | Risk Control (this row), flagged for Unresolved Questions below — not a blocker for this item |
| Incomplete producer audit: a further unguarded `events.NewNotificationEvent` call site reproduces the same dead-link bug this plan exists to fix | Epic 2/3/5 close the three session-scoped, currently-unguarded producers identified by name so far (`OnItemAdded`, `autonomous_orchestration_service.go`'s generic notifier, `wireRateLimitCallbacks`). This plan-repair pass's grep of all `events.NewNotificationEvent(` call sites also surfaced at least two more session-scoped, currently-unguarded producers **not** fixed by this plan and left as an explicitly out-of-scope follow-up: `CapacityMonitor.handleTransitionTrigger` (`server/services/capacity_monitor.go:288`, "Capacity Alert" notice keyed on `snap.UUID`/`snap.Title`, no `Hidden` check) and `SessionService.UpdateSession`'s steer-message confirmation (`server/services/session_service.go:1763-1768`, "Steering input sent," gated behind an explicit user-initiated `SteerMessage` RPC call so practically low-risk against a `Hidden` session, but structurally the same unguarded shape). No structural, compiler-enforced guard exists against *any* future producer (including these two) being added/staying unguarded — this remains a code-review checklist item, not a type-system guarantee, and closing these two specific instances is out of this item's scope | Epic 2, Epic 3, Epic 5 |

---

## Unresolved Questions

None blocking implementation. Two tunables left as documented defaults rather than hard
constraints: `pruneOrphanedMinUptime = 5 * time.Minute` (Task 4.3.1a) is a defensive margin, not a
value derived from a measured startup-reload duration — if a future, much-larger instance count
made `BuildRuntimeDeps`'s synchronous `LoadInstances()` call itself take longer than 5 minutes
(implausible at current scale — it's a single JSON/SQLite read, not per-instance I/O), this
constant would need to grow. Similarly, `orphanPruneInterval = 1 * time.Minute` (Task 4.2.1b) is a
proportionate-feeling default for how often the batch existence-fetch runs, not a value derived
from measuring `ListInstanceData()`'s actual cost at scale — if that call ever becomes expensive
enough that even once-a-minute is too frequent, this constant would need to grow. Flagging both
for awareness, not blocking.

A third item, surfaced by Phase 4's Product Triad Review (UX lens): a `Hidden` review session that
is still alive but hung/silent (`ReasonIdle`/`ReasonStale`, not yet erroring, not yet exited) has no
durable stuck-detection surface after this fix ships — full suppression of these two reasons is
required by AC1's literal text, not a design choice this plan could soften without contradicting the
acceptance criteria. See the corresponding Risk Control row above for the full analysis and a
recommended (separate, out-of-scope) follow-up backlog item to close this residual gap durably.
Not blocking this item — before this fix, the only "signal" for this exact case was the same
noisy, dead-linked notification the backlog item exists to eliminate, so this is a documented
trade-off rather than a regression in actionable monitoring.

---

## Dependency Visualization

```
Epic 1: Determine() reason-scoped Hidden gate (AC1, structural)
  Story 1.1: Determine() end-of-function reason-scoped gate ──┐
    Task 1.1.1a → 1.1.1b                                       │
  Story 1.2: StartupScanner regression test only, no source change ┤ (no longer a code dependency
    Task 1.2.1a                                                 │  on 1.1 — see Design Decision 5 —
                                                                 │  but the test still exercises 1.1's fix)
                                                                 ▼
Epic 2: OnItemAdded suppression + item_id enrichment (AC1 defense-in-depth, AC2)
  depends on nothing in Epic 1 (separate call path) but ships together
  Story 2.1: resolve inst/hiddenSession + ItemSession linkage lookup
    Task 2.1.1a → 2.1.1b → 2.1.1c
  Story 2.2: gate publish (reason-scoped) + stamp item_id/session_scoped metadata ── depends on 2.1
    Task 2.2.1a → 2.2.1b
  Story 2.3: regression tests (AC4 primary) ── depends on 2.2
    Task 2.3.1a → 2.3.1b → 2.3.1c → 2.3.1d
  Story 2.4: headless-triage negative-proof regression test (AC4, Finding 3) ── independent of 2.1-2.3
    Task 2.4.1a
                                                     │
Epic 3: autonomous_orchestration_service.go fixes (AC1/AC2, independent of Epic 1/2 files)
  Story 3.1: "Triage stuck" metadata fix
    Task 3.1.1a
  Story 3.2: generic done/stuck notifier — linkedItemID + Hidden gate + metadata
    Task 3.2.1a → 3.2.1b
  Story 3.3: tests
    Task 3.3.1a
                                                     │
                                                     ▼
Epic 4: NotificationRecord.SessionScoped + PruneOrphaned (AC3)
  depends on Epic 2's Task 2.2.1b (session_scoped metadata key must exist before eventToRecord
  reads it), Epic 3's Task 3.2.1b (same key, second producer), and Epic 5's Task 5.1.1a (same key,
  third producer)
  Story 4.1: SessionScoped field + eventToRecord wiring
    Task 4.1.1a → 4.1.1b
  Story 4.2: PruneOrphaned + SetSessionExistenceLookup + gated enforceRetention hook
    Task 4.2.1a → 4.2.1b
  Story 4.3: wire in server.go with uptime guard ── depends on 4.2
    Task 4.3.1a
  Story 4.4: tests ── depends on 4.1, 4.2
    Task 4.4.1a → 4.4.1b → 4.4.1c → 4.4.1d

Epic 5: wireRateLimitCallbacks Hidden gate + metadata (AC1/AC2, Finding 1 — third unguarded producer)
  independent of Epics 1-2 in every file it touches (server/services/session_service.go); depends
  only on Epic 2's Task 2.1.1c (events.SessionScopedMetadata helper)
  Story 5.1: gate onDetected/onRecovery on inst.Hidden + stamp item_id/session_scoped metadata
    Task 5.1.1a → 5.1.1b
  Story 5.2: tests
    Task 5.2.1a

Epic 1 and Epic 2 touch disjoint files (session/review_queue_determiner.go + session/startup_scanner.go
vs. server/review_queue_manager.go) and can be implemented/reviewed in parallel. Epic 3 is
independent of Epics 1-2 in every file it touches (different file, different struct) and can also
proceed in parallel, **except** for one narrow dependency: Epic 3's Task 3.2.1b calls
`events.SessionScopedMetadata`, defined by Epic 2's Task 2.1.1c (`pkg/events`/`server/events`,
shared by both epics precisely so `"session_scoped"` is not a duplicated string literal — see the
architecture-review Concern fix). Task 2.1.1c has no dependency on the rest of Epic 2 and is cheap
to land first/standalone if Epic 3 needs to start before Epic 2's other tasks are done. Epic 5 has
the exact same narrow dependency on Task 2.1.1c and is otherwise independent of every other epic.
Epic 4 is the epic with the broadest ordering dependency: it needs the `events.SessionScopedMetadata`
helper (Task 2.1.1c) landed and consumed by all three real producers (Epic 2's Task 2.2.1a, Epic 3's
Task 3.2.1b, Epic 5's Task 5.1.1a) before `eventToRecord`/`PruneOrphaned` (Task 4.1.1b) have anything
meaningful to read.
```

---

## Phase 1 (single phase — all epics are small; see Dependency Visualization for parallelism)

### Epic 1: Close the `Determine()`/`StartupScanner` structural bypass (AC1)

**Goal**: A `Hidden` instance can never produce a `DetectionActionAdd` result from `Determine()`
for `ReasonTaskComplete`/`ReasonIdle`/`ReasonStale`, regardless of which caller
(`ReviewQueuePoller.checkSession` or `StartupScanner.Scan`) invokes it — closing the confirmed
reproducible bypass where a service restart lets `StartupScanner.Scan` call `Determine()` on a
still-`Hidden`, still-running review session with zero `Hidden` check (architecture.md Q1) — while
narrowly *preserving* `ReasonErrorState`/`ReasonTestsFailing` notifications even when `Hidden`, per
Design Decision 1's narrowing (Phase 4 pre-mortem Failure #3: no other durable detector watches a
still-alive, stuck-in-error `Hidden` review session; see Design Decision 1 for the full
justification and the citation of `reconcileStuckReviewItems`/`markAbandonedReview` as the
mechanism that *does* cover the exited/zombie sub-case).

#### Story 1.1: Add `Determine()`'s reason-scoped `Hidden` gate
**As an** operator, **I want** `Determine()` itself to refuse to flag a `Hidden` instance as
`TaskComplete`/`Idle`/`Stale`, while still flagging a genuine error/tests-failing condition even
for a `Hidden` instance, **so that** no current or future caller of `Determine()` can re-introduce
the `StartupScanner` bypass by accident, without silencing the one signal that watches an
alive-but-stuck `Hidden` review session.
**Acceptance Criteria**:
- `DefaultStatusDeterminer.Determine` (`session/review_queue_determiner.go:288-300`) forces
  `action = DetectionActionSkip` when `inst.Hidden && action == DetectionActionAdd && reason ∈
  {ReasonTaskComplete, ReasonIdle, ReasonStale}`, applied immediately before the function's single
  terminal `return DetectionResult{...}` (see Design Decision 4's revised code).
- **Given** an `Instance{Title: "review:153f8eac", UUID: "aaaa1111-2222-3333-4444-555566667777",
  Hidden: true}` (matching `SpawnReviewSession`'s title convention `"review:"+item.ID[:8]` for
  backlog item `153f8eac-c454-4fa3-a8f4-83b070b9a035`), with
  `statusInfo.ClaudeStatus == detection.StatusSuccess` and `statusInfo.IsControllerActive ==
  true` (i.e., every input that would otherwise produce `ReasonTaskComplete`),
  **when** `DefaultStatusDeterminer.Determine(inst, "", statusInfo, detector)` is called,
  **then** it returns `DetectionResult{Action: DetectionActionSkip, Reason: ReasonTaskComplete}`.
- **Given** the same `Hidden: true` instance, but `statusInfo.ClaudeStatus ==
  detection.StatusError` (i.e., every input that would otherwise produce `ReasonErrorState`),
  **when** `Determine()` is called, **then** it returns `DetectionResult{Action:
  DetectionActionAdd, Reason: ReasonErrorState}` — the `Hidden` gate does **not** suppress this
  reason, proving the narrowing's safety-net case.
- Existing `Determine()` tests for non-Hidden instances (e.g. approval/error/idle detection)
  pass unmodified.
**Files**: `session/review_queue_determiner.go`, `session/review_queue_determiner_test.go`

##### Task 1.1.1a: Add the reason-scoped `Hidden` gate to `Determine()`'s final return (~4 min)
- In `session/review_queue_determiner.go`, insert the gate shown in Design Decision 4's revised
  code, immediately before the existing terminal `return DetectionResult{...}` (currently around
  line 293) — **not** as a first-statement early-return (that shape can no longer work now that
  the gate depends on `reason`, which isn't known until the rest of the function has run).
- Files: `session/review_queue_determiner.go`

##### Task 1.1.1b: Add `TestDetermine_ReturnsSkip_When_InstanceHiddenAndReasonIsTaskCompleteIdleOrStale` + a `ReasonErrorState` safety-net case (~7 min)
- In `session/review_queue_determiner_test.go`, add a table-driven or standalone test
  constructing a bare `&Instance{Title: "review:153f8eac", UUID:
  "aaaa1111-2222-3333-4444-555566667777", Hidden: true}` (no live tmux, following the
  `session/review_queue_reactive_test.go` bare-`&Instance{}` pattern) with `InstanceStatusInfo{
  IsControllerActive: true, ClaudeStatus: detection.StatusSuccess }` and assert
  `result.Action == DetectionActionSkip`. Add cases for `ClaudeStatus: detection.StatusIdle`-shaped
  idle timeout and staleness (`ReasonIdle`/`ReasonStale`), all asserting `DetectionActionSkip`.
  **Then add the safety-net case**: the same `Hidden: true` instance with `ClaudeStatus:
  detection.StatusError`, asserting `result.Action == DetectionActionAdd && result.Reason ==
  ReasonErrorState` — proving the narrowing does not over-suppress. Add a parallel case for
  `detection.StatusTestsFailing` → `ReasonTestsFailing` staying `DetectionActionAdd`.
- Files: `session/review_queue_determiner_test.go`

#### Story 1.2: `StartupScanner.Scan` regression coverage (no source change)
**As a** maintainer, **I want** a test proving `StartupScanner.Scan` still delegates correctly to
`Determine()`'s narrowed `Hidden` gate for a `Hidden` instance loaded at startup, **so that** the
specific reproducible bypass (architecture.md Q1) stays closed **without** reintroducing a
separate, reason-blind `Hidden` skip at `StartupScanner.Scan`'s own call site.
**Acceptance Criteria**:
- **No source change to `session/startup_scanner.go`** (see Design Decision 5, revised): the plan
  originally proposed adding `|| inst.Hidden` to `Scan`'s skip condition at line 35, but that
  addition was dropped once suppression became reason-scoped — it would skip `Determine()`
  entirely for a `Hidden` instance regardless of reason, silently defeating Story 1.1's
  `ReasonErrorState`/`ReasonTestsFailing` safety net for exactly the startup-scan path Design
  Decision 5 was originally trying to protect.
- **Given** `instances := []*Instance{{Title: "review:153f8eac", Hidden: true,
  /* started, unpaused */}}` with fixture status content that would otherwise produce
  `ReasonTaskComplete`, **when** `(&StartupScanner{}).Scan(instances, queue)` is called, **then**
  `queue.Add` is never invoked for that instance (via `Determine()`'s narrowed gate, not via a
  separate startup-scanner-level skip).
- **Given** the same `Hidden: true` instance but with fixture status content that would otherwise
  produce `ReasonErrorState`, **when** `Scan` is called, **then** `queue.Add` **is** invoked —
  proving `StartupScanner.Scan` does not independently block the safety-net reason at its own
  call site.
**Files**: `session/startup_scanner_test.go`

##### Task 1.2.1a: Add `TestScan_SkipsHiddenInstance_ForSuppressedReasonsOnly` (~6 min)
- In `session/startup_scanner_test.go` (create if it does not already exist, following the
  `StatusProvider`/`ContentProvider` fake-interface pattern from
  `session/review_queue_reactive_test.go`), build a `Hidden: true`, `Started()`-true instance with
  a fake `StatusProvider`/`ContentProvider` returning `ReasonTaskComplete`-shaped status, call
  `Scan`, and assert `added == 0`. Add a second case with `ReasonErrorState`-shaped status and
  assert `added == 1` — together proving `Scan`'s unmodified call to `Determine()` produces the
  narrowed result, not an over-broad startup-level skip.
- Files: `session/startup_scanner_test.go`

---

### Epic 2: `OnItemAdded` suppression + `item_id` enrichment (AC1 defense-in-depth, AC2)

**Goal**: Independent of Epic 1's structural fix, `OnItemAdded` itself refuses to publish a
`ReasonTaskComplete`/`ReasonIdle`/`ReasonStale` notification for any `ReviewItem` resolved to a
`Hidden` `Instance` (while still publishing for `ReasonErrorState`/`ReasonTestsFailing`, per Design
Decision 1's narrowing), and stamps `item_id` + `session_scoped` metadata onto every notification
tied to a backlog-linked session — satisfying the requirement that suppression not rely solely on
upstream callers having already filtered Hidden instances out. Story 2.4 additionally closes AC4's
"review/triage" wording for the headless-triage half of the requirement (Finding 3), which turns
out to need a provable-negative test rather than a suppression fix, since that path never
constructs a `session.Instance` and was never reachable by `Determine()`/`OnItemAdded` to begin
with.

#### Story 2.1: Resolve `inst`/`hiddenSession` and the `ItemSession` linkage lookup
**As a** maintainer, **I want** `OnItemAdded` to resolve both "is this session Hidden" and "what
backlog item is this session linked to" in one place, using the instance resolution it already
performs, **so that** the suppression and enrichment logic in Story 2.2 have everything they need
with no additional lookups — **and without ever writing onto the shared `*ReviewItem` itself**
(see the data-race note below).
**Acceptance Criteria**:
- `OnItemAdded` (`server/review_queue_manager.go:319`) captures the resolved `*session.Instance`
  (not just its stable ID string) from the existing `rqm.poller.FindInstance(item.SessionID)`
  call (currently lines 349-353), and derives `hiddenSession := inst != nil && inst.Hidden`.
- A new bounded-timeout call to `rqm.storage.GetItemSessionBySessionUUID(ctx, resolvedID)` is
  added (guarded by `rqm.storage != nil`, mirroring `maybeAutoCreatePR`'s existing nil-guard
  style), using a new `itemSessionLookupTimeout = 2 * time.Second` constant and
  `rqm.baseContext()` (the existing helper at `server/review_queue_manager.go:459-464`). A
  resolved backlog item ID is captured into a new **local** `linkedItemID string` — `item.Metadata`
  itself is never read into, written to, or reassigned by this lookup.
- A lookup failure (including "not found") is handled silently (no `linkedItemID`, no
  suppression from this signal) except a real (non-`ErrNotFound`) error, which is logged at
  `Warn`.
- **Never mutate `item.Metadata` in place, at any point in `OnItemAdded`.** `ReviewQueue.Add()`
  (`session/queue/queue.go:230`) stores this exact `*ReviewItem` pointer into `rq.items` and, after
  releasing `rq.mu` (`queue.go:258`), calls `observer.OnItemAdded(item)` unlocked (`queue.go:262`)
  — the same pointer is independently reachable from a concurrent `WatchReviewQueue` RPC handler
  goroutine via `rqm.queue.List()` → `reviewItemToProto` → `adapters.ReviewItemToProto`, which
  ranges over `item.Metadata` (`server/adapters/review_queue_adapter.go:50`). Writing to
  `item.Metadata` here while that goroutine ranges over the same map is a Go runtime **fatal
  error: concurrent map read and map write** (process crash, not a benign race). `Story 2.2`
  builds an independent local map instead, mirroring `ReviewItemToProto`'s own "Always produce an
  independent copy of Metadata so concurrent RPC calls cannot race on the same underlying map"
  pattern.
**Files**: `server/review_queue_manager.go`

##### Task 2.1.1a: Capture `inst`/`hiddenSession` in `OnItemAdded` (~3 min)
- In `server/review_queue_manager.go`, replace lines 348-353's
  ```go
  resolvedID := item.SessionID
  if rqm.poller != nil {
      if inst := rqm.poller.FindInstance(item.SessionID); inst != nil {
          resolvedID = inst.GetStableID()
      }
  }
  ```
  with:
  ```go
  resolvedID := item.SessionID
  var inst *session.Instance
  if rqm.poller != nil {
      if i := rqm.poller.FindInstance(item.SessionID); i != nil {
          inst = i
          resolvedID = i.GetStableID()
      }
  }
  hiddenSession := inst != nil && inst.Hidden
  var linkedItemID string
  ```
- Files: `server/review_queue_manager.go`

##### Task 2.1.1b: Add `itemSessionLookupTimeout` const + the linkage lookup (~5 min)
- Near the existing `autoCreatePRLookupTimeout`/`autoCreatePRRunTimeout` consts
  (`server/review_queue_manager.go:47-48`), add:
  ```go
  // itemSessionLookupTimeout bounds the synchronous ItemSession lookup added to OnItemAdded's
  // observer callback (Task 2.1.1b). Short because this runs inline in the queue-mutation
  // critical path, not in an async goroutine like maybeAutoCreatePR's 20s lookup timeout.
  itemSessionLookupTimeout = 2 * time.Second
  ```
  Then, immediately after Task 2.1.1a's block (still before the existing
  `if rqm.eventBus != nil && item.Reason != session.ReasonApprovalPending` line), add:
  ```go
  if rqm.storage != nil {
      lookupCtx, cancel := context.WithTimeout(rqm.baseContext(), itemSessionLookupTimeout)
      itemSession, err := rqm.storage.GetItemSessionBySessionUUID(lookupCtx, resolvedID)
      cancel()
      if err != nil {
          if !errors.Is(err, session.ErrNotFound) {
              log.Warn("OnItemAdded: ItemSession lookup failed", "session", resolvedID, "err", err)
          }
      } else if itemSession.BacklogItemID != "" {
          linkedItemID = itemSession.BacklogItemID
      }
  }
  ```
  Note this assigns the **local** `linkedItemID` declared in Task 2.1.1a — `item.Metadata` is
  never touched here (see Story 2.1's data-race note); Story 2.2's `events.SessionScopedMetadata`
  call is the only place `linkedItemID` and any pre-existing `item.Metadata` entries are combined,
  into a brand-new map.
  (`"time"` is already imported in this file for `autoCreatePRRunTimeout`/`baseContext`; add
  `"errors"` to the import block — it is not currently imported.)
- Files: `server/review_queue_manager.go`

##### Task 2.1.1c: Add the shared `events.SessionScopedMetadata` helper + metadata key constants (~5 min)
- Addresses the architecture-review Concern that `"session_scoped"` was otherwise duplicated as
  an independent string literal in three files with no shared constant. In `pkg/events` (e.g. a
  new small file `pkg/events/notification_metadata.go`, or added directly in `types.go` near
  `NewNotificationEvent`), add:
  ```go
  const (
      MetadataKeySessionScoped = "session_scoped"
      MetadataKeyItemID        = "item_id"
  )

  // SessionScopedMetadata builds a fresh metadata map for a session-scoped notification,
  // copying any entries from base (never mutating base — base may be a map read concurrently
  // elsewhere, e.g. session.ReviewItem.Metadata) and adding the session_scoped marker plus
  // item_id when non-empty.
  func SessionScopedMetadata(base map[string]string, itemID string) map[string]string {
      m := make(map[string]string, len(base)+2)
      for k, v := range base {
          m[k] = v
      }
      m[MetadataKeySessionScoped] = "true"
      if itemID != "" {
          m[MetadataKeyItemID] = itemID
      }
      return m
  }
  ```
  Then, in `server/events/forward.go`, forward both constants (alongside the existing
  `EventNotification = pkgevents.EventNotification` const block) and the function (alongside the
  existing `NewNotificationEvent = pkgevents.NewNotificationEvent` var block), so
  `server/review_queue_manager.go` and `server/services/autonomous_orchestration_service.go`
  (both of which import `server/events`, not `pkg/events`, directly) can call
  `events.SessionScopedMetadata(...)` / reference `events.MetadataKeySessionScoped` without a new
  import.
- This task has no dependency on Task 2.1.1a/2.1.1b's `OnItemAdded` changes and can be implemented
  first/independently; Epic 3's Task 3.2.1b and Epic 4's Task 4.1.1b both consume this task's
  output (a narrow, one-file-each dependency — see the updated Dependency Visualization note
  below), so land this task before either.
- Files: `pkg/events/types.go` (or a new `pkg/events/notification_metadata.go`), `server/events/forward.go`

#### Story 2.2: Gate the publish on `hiddenSession` (reason-scoped), stamp `session_scoped` metadata via a local map
**As an** operator, **I want** the Notifications page to never receive a `TaskComplete`/`Idle`/
`Stale` card for a `Hidden` session, while still receiving an `ErrorState`/`TestsFailing` card for
one (Design Decision 1's narrowing), and every genuine session-scoped notification to carry a
positive `session_scoped` signal for AC3's pruner, **so that** AC1 and the AC3 groundwork land
together in the same guarded block — **without mutating the shared `*ReviewItem.Metadata` map**
(Story 2.1's data-race note).
**Acceptance Criteria**:
- A new unexported helper (or inline boolean), e.g.
  `suppressForHidden := hiddenSession && (item.Reason == session.ReasonTaskComplete ||
  item.Reason == session.ReasonIdle || item.Reason == session.ReasonStale)`, computed alongside
  `hiddenSession` (Task 2.1.1a).
- The existing guard `if rqm.eventBus != nil && item.Reason != session.ReasonApprovalPending {`
  (`server/review_queue_manager.go:337`) becomes
  `if rqm.eventBus != nil && item.Reason != session.ReasonApprovalPending && !suppressForHidden {`
  — **not** `&& !hiddenSession` unconditionally (Design Decision 1's narrowing: `ReasonErrorState`/
  `ReasonTestsFailing` must still publish even when `hiddenSession` is true).
- Inside that block, before constructing `notifEvent`, build a **fresh, independent** metadata map
  via `metadata := events.SessionScopedMetadata(item.Metadata, linkedItemID)` — this copies any
  existing `item.Metadata` entries into a new map (never reading concurrently-written state,
  never writing back into `item.Metadata`), sets `MetadataKeySessionScoped` to `"true"`
  unconditionally (since `resolvedID` here is always a real session identifier — either
  `item.SessionID`, the queue-key title, or `inst.GetStableID()` — never a backlog item ID), and
  sets `MetadataKeyItemID` from `linkedItemID` only when non-empty (Story 2.1's lookup result).
- `events.NewNotificationEvent(...)`'s trailing metadata argument becomes this local `metadata`
  map, not `item.Metadata` directly.
- **Given** a real (non-Hidden) work session `Instance{UUID: "bbbb2222-3333-4444-5555-666677778888",
  Hidden: false}` linked via `ItemSessionData{SessionUUID: "bbbb2222-...", SessionRole: "work",
  ItemID: "153f8eac-c454-4fa3-a8f4-83b070b9a035"}`, reaching `ReasonIdle`, **when**
  `OnItemAdded(&session.ReviewItem{SessionID: <title>, Reason: session.ReasonIdle, ...})` runs,
  **then** the published `events.NewNotificationEvent`'s metadata includes
  `{"item_id": "153f8eac-c454-4fa3-a8f4-83b070b9a035", "session_scoped": "true"}`, and
  `item.Metadata` itself is left unchanged (still nil, or whatever it was before the call).
- **Given** the `Hidden: true` review `Instance` from Story 1.1's example, **when**
  `OnItemAdded` is called with a `ReviewItem{Reason: session.ReasonTaskComplete}` resolved to
  that instance, **then** `rqm.eventBus.Publish` is never called.
- **Given** the same `Hidden: true` review `Instance`, **when** `OnItemAdded` is called with a
  `ReviewItem{Reason: session.ReasonErrorState}` resolved to that instance, **then**
  `rqm.eventBus.Publish` **is** called (the narrowing's safety net — see Design Decision 1).
**Files**: `server/review_queue_manager.go`

##### Task 2.2.1a: Update the publish guard (reason-scoped) and build the local `session_scoped` metadata map (~6 min)
- In `server/review_queue_manager.go`, add the `suppressForHidden` computation and change the
  guard at line 337 as described above — a reason-scoped condition, not an unconditional
  `!hiddenSession`. Then, immediately before the existing `notifID := fmt.Sprintf(...)` line, add
  `metadata := events.SessionScopedMetadata(item.Metadata, linkedItemID)`, and change the trailing
  argument of the `events.NewNotificationEvent(...)` call from `item.Metadata` to `metadata`. Do
  **not** assign into `item.Metadata` anywhere in this task — see Story 2.1's data-race note.
- Files: `server/review_queue_manager.go`

##### Task 2.2.1b: Verify existing `OnItemAdded` tests still pass unmodified (~2 min, verification only)
- Confirm `TestOnItemAdded_NotificationUsesStableID` and
  `TestOnItemAdded_NotificationFallsBackToTitleWhenNoMatch`
  (`server/review_queue_manager_test.go:669`, `:717`) require no changes: both assert only on
  `e.SessionID`, never on `Metadata`, and both use `newReactiveQueueTestSetup` (storage `nil`,
  so Task 2.1.1b's lookup block is skipped) with a non-`Hidden` (or absent) instance, so
  `hiddenSession` is `false` in both and the new `session_scoped` key is simply additive and
  unobserved by these tests. Run
  `go test ./server/... -run TestOnItemAdded_Notification` to confirm.
- Files: `server/review_queue_manager_test.go` (read-only verification)

#### Story 2.3: AC4 regression tests (primary)
**As a** future maintainer, **I want** a test proving a `Hidden` review session reaching
`TASK_COMPLETE`/`Idle`/`Stale` produces zero `EventNotification`s on the bus, **so that** this
specific bug class (Notifications page filling with dead-link entries) cannot silently regress.
**Acceptance Criteria**:
- New test(s) in `server/review_queue_manager_test.go`, following
  `TestOnItemAdded_EventBusBehavior_BUG001`'s exact shape (direct `eventBus.Subscribe(ctx)`,
  bounded `select`/`time.After`), using `newReactiveQueueTestSetupWithStorage` so a real
  `ItemSession` row can be created.
- **Given** `poller.SetInstances([]*session.Instance{{Title: "review:153f8eac", UUID:
  "aaaa1111-2222-3333-4444-555566667777", Hidden: true}})` and a corresponding
  `storage.CreateItemSession(ctx, session.ItemSessionData{ItemID: <a real created backlog item
  ID>, SessionUUID: "aaaa1111-...", SessionRole: "review"})`, **when**
  `mgr.OnItemAdded(&session.ReviewItem{SessionID: "review:153f8eac", Reason:
  session.ReasonTaskComplete, Priority: session.PriorityLow, DetectedAt: time.Now()})` is called,
  **then** no `events.EventNotification` arrives on the subscribed channel within 300ms.
- A second sub-test repeats the same shape for `session.ReasonIdle` and `session.ReasonStale`.
- A third sub-test (negative control) repeats the shape with `Hidden: false` and asserts a
  notification **does** arrive — proving the test harness itself would catch a regression that
  over-suppresses.
- A fourth sub-test (narrowing safety-net) repeats the shape with `Hidden: true` but
  `Reason: session.ReasonErrorState`, asserting a notification **does** arrive within 300ms —
  proving Design Decision 1's narrowing (Phase 4 pre-mortem Failure #3) actually holds and this
  reason is never swallowed by the `Hidden` gate.
**Files**: `server/review_queue_manager_test.go`

##### Task 2.3.1a: Add `TestOnItemAdded_SuppressesNotification_When_SessionHidden` (~6 min)
- Add the test described above for `ReasonTaskComplete`, reusing
  `newReactiveQueueTestSetupWithStorage` (`server/review_queue_manager_test.go:777`).
- Files: `server/review_queue_manager_test.go`

##### Task 2.3.1b: Extend to `ReasonIdle`/`ReasonStale` via `t.Run` subtests (~4 min)
- Convert Task 2.3.1a's test into a table/`t.Run`-driven test covering all three reasons.
- Files: `server/review_queue_manager_test.go`

##### Task 2.3.1c: Add the `Hidden: false` negative control (~4 min)
- Add `TestOnItemAdded_PublishesNotification_When_SessionNotHidden_EvenIfBacklogLinked`,
  reusing the same harness with `Hidden: false`, asserting a notification **does** arrive and
  carries `metadata["item_id"]` — proves Epic 2 doesn't over-suppress real sessions.
- Files: `server/review_queue_manager_test.go`

##### Task 2.3.1d: Add the `ReasonErrorState`/`ReasonTestsFailing` narrowing safety-net case (~5 min)
- Add `TestOnItemAdded_PublishesNotification_When_SessionHidden_AndReasonIsErrorStateOrTestsFailing`,
  reusing the same harness with `Hidden: true` but `Reason: session.ReasonErrorState` (and a
  `t.Run` subtest for `session.ReasonTestsFailing`), asserting a notification **does** arrive
  within 300ms — proves Design Decision 1's narrowed suppression scope actually holds in
  `OnItemAdded`, not just in `Determine()` (Story 1.1's equivalent case).
- Files: `server/review_queue_manager_test.go`

#### Story 2.4: Headless-triage negative-proof regression test (AC4 "review/triage" wording, Finding 3)
**As a** future maintainer, **I want** the AC4 regression coverage to also address the
headless-*triage* half of AC4's "review/triage" wording — even though Phase 2/3 research
(features.md/architecture.md) confirmed headless-triage sessions never construct a
`session.Instance` and are therefore structurally invisible to `Determine()`/`OnItemAdded` — **so
that** a future reader isn't left wondering why every AC4 test so far only exercises the
review-session path.

**Context**: `TriggerTriage`'s headless-pool call (`server/services/backlog_service_triage.go:1677`,
the async goroutine at lines ~1805-1937) has **no** `events.NewNotificationEvent`/`eventBus.Publish`
call at all on its success (lines 1870-1936), `callErr != nil` (lines 1855-1868), or parse-failure
(lines 1871-1876) paths — every one of them only logs and, on failure, marks the `ItemSession`
ended. The **only** notification this goroutine can ever publish is
`notifyTriagePersistFailure` (line 1915, defined at lines 229-246), which is already
unconditionally `item_id`-tagged (`map[string]string{"item_id": itemID}`, line 244) and therefore
already immune to the dead-link bug this plan exists to fix — there is no generic, untagged
TASK_COMPLETE/Idle/Stale-shaped notification for this code path to ever suppress. A real,
non-vacuous regression test is nonetheless constructible as a **provable negative**: drive
`TriggerTriage` through its real success and failure paths with a fake headless pool and assert
the exact, small set of notifications published (zero, in the common cases) never includes an
untagged one.

**Acceptance Criteria**:
- A new test in `server/services/backlog_service_triage_test.go`, reusing the existing
  `fakeHeadlessPool` fixture (`server/services/backlog_service_test.go:47`, supports `err` for
  error injection) and the `require.Eventually`-based async-completion-polling pattern already used
  by this file's other `TriggerTriage` tests (e.g. lines 1956, 2006, 2017) to wait for the
  goroutine to finish (poll `ListItemSessions` for the triage `ItemSession`'s `EndedAt` becoming
  non-nil).
- **Given** a `fakeHeadlessPool{err: errors.New("simulated LLM failure")}` wired via
  `svc.SetHeadlessPool(...)`, and an `events.NewEventBus(4)` subscribed before the call, **when**
  `TriggerTriage` is invoked for a real backlog item and the goroutine is awaited via
  `require.Eventually`, **then** zero events arrive on the subscribed channel within a bounded
  window (e.g. 300ms after the `EndedAt` poll succeeds) — proving the `callErr != nil` path
  publishes nothing.
- **Given** a `fakeHeadlessPool` returning malformed (non-JSON) `response` text, **when**
  `TriggerTriage` is invoked and awaited the same way, **then** zero events arrive — proving the
  parse-failure path publishes nothing either.
- **Given** a `fakeHeadlessPool` returning a valid triage result but with the storage fake
  configured to fail `UpdateBacklogItem` (forcing `notifyTriagePersistFailure` to fire), **when**
  `TriggerTriage` is invoked and awaited, **then** exactly one event arrives and its
  `NotificationMetadata["item_id"]` equals the item's ID — proving the one notification this path
  *can* emit is already correctly tagged, never a generic untagged one.
- If, during implementation, any of the above three cases turns out non-constructible without an
  artificial/contrived harness (e.g. a fake storage layer this test file does not already support),
  the task falls back to an explicit **"Verification via code inspection"** subsection appended to
  this story in `plan.md`, citing the exact line ranges above (goroutine body, `callErr`/
  `parseErr`/`marshalErr` branches, `notifyTriagePersistFailure`'s unconditional `item_id` tag) as
  the basis for the claim that no untagged notification is reachable from this path — not silence.
**Files**: `server/services/backlog_service_triage_test.go`

##### Task 2.4.1a: Add `TestTriggerTriage_NeverPublishesUntaggedNotification_OnHeadlessPoolFailureOrSuccess` (~10 min)
- Implement the three sub-cases described above (LLM call error, parse failure, persist failure)
  as `t.Run` subtests in `server/services/backlog_service_triage_test.go`, following this file's
  existing `fakeHeadlessPool` + `require.Eventually` conventions. If a sub-case cannot be
  constructed without contriving an artificial failure injection point not already supported by
  the test fixtures, skip only that sub-case and instead add a **"Verification via code
  inspection"** comment block directly above the test function, citing
  `server/services/backlog_service_triage.go`'s exact line ranges for the uncovered branch and
  explaining why no `events.NewNotificationEvent`/`eventBus.Publish` call exists on it.
- Files: `server/services/backlog_service_triage_test.go`

---

### Epic 3: `autonomous_orchestration_service.go` fixes (AC1/AC2)

**Goal**: Fix the two documented `nil`-metadata gaps in the second, independent generic-completion
notifier, and add the `Hidden` gate to the one call site that functionally overlaps AC1's intent.

#### Story 3.1: "Triage stuck" metadata fix
**As an** operator, **I want** the "Triage stuck" notification to carry `item_id` like every
other notification in this file, **so that** it also gets "View in Backlog" routing instead of a
dead link, even though this branch is not currently reachable in production.
**Acceptance Criteria**:
- The `events.NewNotificationEvent(...)` call inside the `SessionRoleTriage`/`!outcome.Done`
  branch (`server/services/autonomous_orchestration_service.go`, ~line 310-320) has its trailing
  `nil` argument replaced with `map[string]string{"item_id": item.ID}`.
- **Given** `item := &session.BacklogItemData{ID: "153f8eac-c454-4fa3-a8f4-83b070b9a035", Title:
  "Fix the thing"}` and `outcome := session.AutonomousDriverOutcome{Done: false, Reason: "turn cap
  reached"}` for an `is.Role == session.SessionRoleTriage` session, **when**
  `onAutonomousDriverComplete` reaches this branch, **then** the published notification's
  metadata is `{"item_id": "153f8eac-c454-4fa3-a8f4-83b070b9a035"}` (previously `nil`).
**Files**: `server/services/autonomous_orchestration_service.go`

##### Task 3.1.1a: Replace `nil` with `{"item_id": item.ID}` at the triage-stuck call site (~2 min)
- In `server/services/autonomous_orchestration_service.go`, locate the `a.bus.Publish(events.NewNotificationEvent(...))`
  call inside the `case session.SessionRoleTriage:` / `if !outcome.Done` branch and change its
  final `nil` argument to `map[string]string{"item_id": item.ID}`.
- Files: `server/services/autonomous_orchestration_service.go`

#### Story 3.2: Generic done/stuck notifier — `linkedItemID` threading + `Hidden` gate + metadata
**As an** operator, **I want** the generic "Autonomous fix complete"/"Autonomous fix stuck"
notification to carry `item_id` when the session is backlog-linked, and to never fire for a
`Hidden` session, **so that** this second notifier doesn't reopen the same dead-link problem
Epic 1/2 close for the primary review-queue notifier.
**Acceptance Criteria**:
- A new outer-scope `var linkedItemID string` is declared alongside the existing
  `var statusTransitionErr error` (near `server/services/autonomous_orchestration_service.go:262`),
  and set to `item.ID` at the point inside the nested `GetItemSessionBySessionUUID`/
  `GetBacklogItem` block where `item` is first successfully resolved.
- The final `a.bus.Publish(events.NewNotificationEvent(...))` call (~line 540) is wrapped in
  `if !inst.Hidden { ... }`, and its trailing `nil` metadata argument becomes
  `events.SessionScopedMetadata(nil, linkedItemID)` — the same shared helper Task 2.2.1a uses,
  rather than a second hand-built `map[string]string{"session_scoped": "true", ...}` literal (see
  the architecture-review Concern fix in Design Decision 6).
- **Given** `inst := &session.Instance{UUID: "cccc3333-4444-5555-6666-777788889999", Hidden:
  true}` (a hypothetical future Hidden autonomous-driver-run instance) and
  `outcome := session.AutonomousDriverOutcome{Done: true}`, **when**
  `onAutonomousDriverComplete("some-hidden-session", outcome)` runs, **then** the generic
  done/stuck `a.bus.Publish` call is never reached.
- **Given** a non-Hidden autonomous work session linked to backlog item
  `"153f8eac-c454-4fa3-a8f4-83b070b9a035"`, **when** the driver completes with `outcome.Done ==
  true`, **then** the published notification's metadata is
  `{"item_id": "153f8eac-c454-4fa3-a8f4-83b070b9a035", "session_scoped": "true"}` (previously
  `nil`).
**Files**: `server/services/autonomous_orchestration_service.go`

##### Task 3.2.1a: Add `linkedItemID` and set it inside the nested lookup block (~4 min)
- In `server/services/autonomous_orchestration_service.go`, add `var linkedItemID string` next
  to `var statusTransitionErr error`; inside the block where `item, itemErr :=
  concreteStorage.GetBacklogItem(ctx, is.BacklogItemID)` succeeds, add `linkedItemID = item.ID`.
- Files: `server/services/autonomous_orchestration_service.go`

##### Task 3.2.1b: Gate the generic notifier on `!inst.Hidden` and build its metadata via the shared helper (~4 min)
- Wrap the final `a.bus.Publish(events.NewNotificationEvent(sessionUUID, instanceName, ...))`
  call in `if !inst.Hidden { ... }`, replacing the trailing `nil` with
  `events.SessionScopedMetadata(nil, linkedItemID)` as described in the story's acceptance
  criteria — not a second hand-built map literal.
- Files: `server/services/autonomous_orchestration_service.go`

#### Story 3.3: Tests for Epic 3
**Acceptance Criteria**:
- A test for Task 3.1.1a's metadata fix and a test for Task 3.2.1b's `Hidden` gate + metadata,
  using this file's existing test fixture pattern (search for existing
  `onAutonomousDriverComplete` tests, if any, to match harness style — otherwise construct a
  minimal `AutonomousOrchestrationService` with a fake `instanceFinder`/`storageGetter`/`bus`).
**Files**: `server/services/autonomous_orchestration_service_test.go`

##### Task 3.3.1a: Add tests for both fixed call sites (~8 min)
- Add `TestOnAutonomousDriverComplete_StampsItemID_When_TriageStuck` and
  `TestOnAutonomousDriverComplete_SuppressesGenericNotification_When_InstanceHidden` (plus a
  non-Hidden positive-metadata case) to
  `server/services/autonomous_orchestration_service_test.go`.
- Files: `server/services/autonomous_orchestration_service_test.go`

---

### Epic 4: `NotificationRecord.SessionScoped` + `PruneOrphaned` (AC3)

**Goal**: Notifications whose referenced session no longer exists are pruned, without deleting
any item-scoped notification whose `SessionID` happens to collide in format with a session UUID.

#### Story 4.1: `SessionScoped` field + `eventToRecord` wiring
**As a** maintainer, **I want** the persisted `NotificationRecord` to carry an explicit,
producer-set signal distinguishing "this SessionID is a real session" from "this SessionID is
actually a backlog item ID," **so that** AC3's pruner has a positive, non-inferred discriminator
(ADR-001).
**Acceptance Criteria**:
- `NotificationRecord` (`server/notifications/store.go:34-52`) gains
  `SessionScoped bool \`json:"session_scoped,omitempty"\`` as its final field.
- `eventToRecord` (`server/notifications/subscriber.go:152-163`) sets
  `SessionScoped: event.NotificationMetadata[events.MetadataKeySessionScoped] == "true"` (the
  exported constant from Task 2.1.1c, not a raw string literal) in the returned
  `*NotificationRecord`.
- **Given** an `*events.Event` with `NotificationMetadata: map[string]string{"session_scoped":
  "true", "item_id": ""}`, **when** `eventToRecord(event)` runs, **then** the returned record has
  `SessionScoped == true`.
- **Given** an `*events.Event` from `backlog_notifier.go`'s `EventBusNotifier.Notify` (no
  `session_scoped` key set), **when** `eventToRecord(event)` runs, **then** the returned record
  has `SessionScoped == false` (zero value).
**Files**: `server/notifications/store.go`, `server/notifications/subscriber.go`

##### Task 4.1.1a: Add `SessionScoped` to `NotificationRecord` (~2 min)
- In `server/notifications/store.go`, add the field after `LastOccurredAt` (line 51) in the
  `NotificationRecord` struct.
- Files: `server/notifications/store.go`

##### Task 4.1.1b: Populate `SessionScoped` in `eventToRecord` (~3 min)
- In `server/notifications/subscriber.go`, add `SessionScoped:
  event.NotificationMetadata[events.MetadataKeySessionScoped] == "true",` to the
  `&NotificationRecord{...}` literal returned by `eventToRecord` (currently lines 152-163) —
  reference the exported constant from Task 2.1.1c, not a raw `"session_scoped"` string literal,
  so producer and consumer cannot silently diverge on the key's spelling.
- Files: `server/notifications/subscriber.go`

#### Story 4.2: `PruneOrphaned` + `SetSessionExistenceLookup` (batch) + gated `enforceRetention` hook
**As an** operator, **I want** stale session-scoped notifications automatically removed on the
existing retention pass, **so that** the Notifications page doesn't accumulate dead-link entries
for up to 7 days — **without turning every single `Append()` call into an O(eligible-records) DB
query storm while the store's write lock is held** (the architecture-review and adversarial-review
blocker this story's original per-record-predicate design triggered independently in both
reviews).
**Acceptance Criteria**:
- New exported method, mirroring `Clear`'s locking/save shape
  (`server/notifications/store.go:306-331`), but taking a **batch** existence-lookup function
  (called exactly once per call, not once per record):
  ```go
  // PruneOrphaned removes records that are positively marked session-scoped
  // (SessionScoped==true, see ADR-001), carry no item_id (Metadata["item_id"] == ""), and whose
  // SessionID is absent from existingSessionIDs()'s returned set. existingSessionIDs is called
  // exactly ONCE per call (a single batch fetch), never once per record — see
  // pruneOrphanedRecords. Returns the number of records removed.
  func (s *NotificationHistoryStore) PruneOrphaned(existingSessionIDs func() map[string]struct{}) (int, error) {
      s.mu.Lock()
      defer s.mu.Unlock()
      removed := s.pruneOrphanedRecords(existingSessionIDs)
      if removed > 0 {
          if err := s.saveToDisk(); err != nil {
              return removed, err
          }
      }
      return removed, nil
  }

  // pruneOrphanedRecords assumes s.mu is already held by the caller (Append's enforceRetention
  // path, or PruneOrphaned's own lock above). Calls existingSessionIDs() exactly once (a single
  // batch fetch, e.g. storage.ListInstanceData()) and checks each candidate record via in-memory
  // map membership — NOT one existence-check call per record, which would re-run a full
  // multi-edge-eager-loaded ent query per eligible record on every Append (the N+1-under-lock
  // this design replaced; see architecture-review.md / adversarial-review.md).
  func (s *NotificationHistoryStore) pruneOrphanedRecords(existingSessionIDs func() map[string]struct{}) int {
      if existingSessionIDs == nil {
          return 0
      }
      existing := existingSessionIDs()
      if existing == nil {
          // The lookup was not ready to judge existence this pass (e.g. still inside
          // pruneOrphanedMinUptime, or the batch fetch itself failed) — treat as "prune
          // nothing," never as "nothing exists" (a nil map is a distinct sentinel from a
          // real, merely-empty map.Get(map[string]struct{}{})).
          return 0
      }
      var kept []*NotificationRecord
      removed := 0
      for _, r := range s.records {
          if r.SessionScoped && r.Metadata["item_id"] == "" {
              if _, ok := existing[r.SessionID]; !ok {
                  removed++
                  continue
              }
          }
          kept = append(kept, r)
      }
      s.records = kept
      return removed
  }
  ```
- A new unexported field `existenceChecker func() map[string]struct{}` and setter
  `SetSessionExistenceLookup(fn func() map[string]struct{})` (locking, mirrors
  `SetNotificationStore`'s late-wiring style) are added — replacing the earlier per-record
  `exists func(sessionID string) bool` shape and its `SetSessionExistenceChecker` setter.
- A new unexported field `lastOrphanPruneAt time.Time` and `const orphanPruneInterval = 1 *
  time.Minute` are added.
- `enforceRetention()` (`server/notifications/store.go:437-454`) gains a **gated** call to the
  orphan sweep, immediately after its existing (unconditional) age/count trim:
  ```go
  if s.existenceChecker != nil && time.Since(s.lastOrphanPruneAt) >= orphanPruneInterval {
      s.lastOrphanPruneAt = now // `now` is already computed at the top of enforceRetention
      if removed := s.pruneOrphanedRecords(s.existenceChecker); removed > 0 {
          log.Info("NotificationHistoryStore: pruned orphaned records", "count", removed)
      }
  }
  ```
  This decouples the batch fetch (a real DB query) from firing on literally every `Append()`
  (roughly every 500ms under the subscriber's coalesce interval) — it now runs at most once per
  `orphanPruneInterval`, reusing the same "cheap, coarse, in-memory-only" cadence philosophy as
  the pre-existing age/count trim (which stays unconditional since it costs nothing extra).
- **Given** a stored `NotificationRecord{ID: "review-queue-review:153f8eac-1690000000000",
  SessionID: "cccc3333-4444-5555-6666-777788889999", SessionScoped: true, Metadata: map[string]string{}}`
  (no `item_id`) whose session was deleted, and a stub `existingSessionIDs` returning
  `map[string]struct{}{}` (i.e. not containing `"cccc3333-..."`), **when**
  `PruneOrphaned(existingSessionIDs)` is called, **then** the record is removed and the returned
  count is `1`, and `existingSessionIDs` was called exactly once.
- **Given** a second stored record with `SessionScoped: true`, `Metadata: {"item_id":
  "153f8eac-..."}`, and the same dead `SessionID`, **when** the same `PruneOrphaned(existingSessionIDs)`
  call runs, **then** that record is **kept** (has an alternate "View in Backlog" navigation
  target).
- **Given** a third stored record with `SessionScoped: false` (an item-scoped
  rework-cap-hit notification whose `SessionID` happens to be a UUID that also is not in the
  returned set), **when** `PruneOrphaned(existingSessionIDs)` runs, **then** that record is
  **kept** (never eligible — the SessionID-overload trap ADR-001 exists to avoid).
- **Given** `existingSessionIDs` returns `nil` (the `pruneOrphanedMinUptime` or fetch-failure
  case), **when** `PruneOrphaned(existingSessionIDs)` or the gated `enforceRetention()` path runs,
  **then** zero records are removed regardless of how many would otherwise be eligible.
**Files**: `server/notifications/store.go`

##### Task 4.2.1a: Implement `PruneOrphaned`/`pruneOrphanedRecords`/`SetSessionExistenceLookup` (~6 min)
- Add the code shown above to `server/notifications/store.go`, plus the `existenceChecker` and
  `lastOrphanPruneAt` fields on the `NotificationHistoryStore` struct and the
  `orphanPruneInterval` const.
- Files: `server/notifications/store.go`

##### Task 4.2.1b: Hook the gated orphan sweep into `enforceRetention` (~3 min)
- In `enforceRetention()` (`server/notifications/store.go:437-454`), after the existing
  `MaxNotifications` trim, add the interval-gated block shown above (reusing the function's
  existing `now := time.Now()` local rather than calling `time.Now()` again).
- Files: `server/notifications/store.go`

#### Story 4.3: Wire the existence lookup in `server.go` via a `Server.startedAt` field
**As an** operator, **I want** the pruner backed by the real, durable instance-record store,
gated against the post-restart reload window, **so that** AC3 works in production without
mass-pruning notifications for sessions that still exist but haven't finished reconciling yet —
**and I want this to actually compile**, which the plan's original closure (referencing a
`NewServer`-local `startTime` from inside `wireDepsIntoServer`, a different function that
`NewServerWithDeps` can also reach without ever computing a `startTime` at all) did not.
**Acceptance Criteria**:
- `Server` (`server/server.go:44-60`) gains a new field: `startedAt time.Time`.
- `newServerBase` (`server/server.go:65-85`) — the function both `NewServer` (line 107) and
  `NewServerWithDeps` (line 127) call before either does anything server-specific — sets
  `srv.startedAt = time.Now()` once, immediately after constructing `srv`.
- `wireDepsIntoServer` (`server/server.go:138`, where `notifStore` is actually constructed —
  **not** `NewServer`, which never reaches this closure's construction point) references
  `srv.startedAt`, not a function-local `startTime`.
- Immediately after `notifStore, storeErr = notifications.NewNotificationHistoryStore(...)`
  succeeds (`server/server.go:203-211`, inside the `else` branch), add a call to
  `notifStore.SetSessionExistenceLookup(...)` using a closure over `storage` (already bound at
  `server/server.go:172`) and `srv.startedAt`:
  ```go
  const pruneOrphanedMinUptime = 5 * time.Minute
  notifStore.SetSessionExistenceLookup(func() map[string]struct{} {
      if time.Since(srv.startedAt) < pruneOrphanedMinUptime {
          // Defensive margin: not ready to judge existence yet. nil is a distinct sentinel
          // from "the real set, which happens to be empty" — pruneOrphanedRecords treats nil
          // as "skip this pass," never as "nothing exists" (which would mass-prune every
          // session-scoped record in the store).
          return nil
      }
      all, err := storage.ListInstanceData()
      if err != nil {
          log.Warn("SetSessionExistenceLookup: ListInstanceData failed; skipping this prune pass", "err", err)
          return nil
      }
      ids := make(map[string]struct{}, len(all)*2)
      for i := range all {
          ids[all[i].GetStableID()] = struct{}{}
          if all[i].Title != "" {
              ids[all[i].Title] = struct{}{}
          }
      }
      return ids
  })
  ```
  `ListInstanceData()` is called exactly once per invocation of this closure (i.e. once per gated
  prune pass, per `orphanPruneInterval` — not once per notification record), matching both
  `all[i].GetStableID()` and `all[i].Title` into the set so membership checks behave the same
  as `InstanceData.MatchesID`'s existing two-way match (`session/storage.go:398-403`).
- **Given** a server less than 5 minutes past `startedAt`, **when** the existence-lookup closure
  is invoked, **then** it returns `nil` without calling `storage.ListInstanceData()` at all.
- **Given** a server past `pruneOrphanedMinUptime` uptime with two stored instances (stable IDs
  `"bbbb2222-..."` and `"cccc3333-..."`), **when** the closure is invoked, **then** it returns a
  set containing both stable IDs (and both titles, if set).
**Files**: `server/server.go`

##### Task 4.3.1a: Add `Server.startedAt`, set it in `newServerBase`, and wire `SetSessionExistenceLookup` from `wireDepsIntoServer` (~6 min)
- In `server/server.go`, add `startedAt time.Time` to the `Server` struct (near `addr`/`httpServer`),
  set it in `newServerBase` right after `srv := &Server{...}` is constructed (before
  `srv.addr.Store(&addr)`), and remove any reliance on a function-local `startTime` for this
  purpose — `NewServer`'s own existing `startTime := time.Now()` (line 111) is unrelated
  dependency-build timing instrumentation and is untouched by this task.
- Add the code shown above inside the existing `if storeErr != nil { ... } else { ... }` block at
  `server/server.go:203-211` (inside `wireDepsIntoServer`), referencing `srv.startedAt`.
- Files: `server/server.go`

#### Story 4.4: Tests for Epic 4
**Acceptance Criteria**:
- Unit tests for `PruneOrphaned` covering the four Given-When-Then cases in Story 4.2's
  acceptance criteria (orphaned-and-eligible removed with the batch fetch called exactly once,
  backlog-linked-and-eligible kept, not-session-scoped-and-eligible kept, `nil` existence-set
  prunes nothing).
- A test proving the orphan sweep inside `enforceRetention()` is cadence-gated (does not re-fetch
  on every `Append()`).
- A test for `eventToRecord`'s new `SessionScoped` population (positive and negative case).
**Files**: `server/notifications/store_test.go`, `server/notifications/subscriber_test.go`

##### Task 4.4.1a: Add `TestPruneOrphaned_RemovesEligibleRecord_KeepsItemLinkedAndNonSessionScoped` (~6 min)
- In `server/notifications/store_test.go`, construct a store, `Append` the three records
  described in Story 4.2's acceptance criteria, call `PruneOrphaned` with a stub
  `existingSessionIDs func() map[string]struct{}` that returns a set not containing the dead
  session ID (and asserts, via a call counter, that it was invoked exactly once), and assert the
  exact kept/removed set.
- Files: `server/notifications/store_test.go`

##### Task 4.4.1b: Add `TestPruneOrphaned_PrunesNothing_When_ExistingSessionIDsReturnsNil` (~3 min)
- In `server/notifications/store_test.go`, `Append` an eligible orphaned record, call
  `PruneOrphaned` with a stub `existingSessionIDs` that returns `nil`, and assert the record is
  kept and the returned count is `0` — proving the `pruneOrphanedMinUptime`/fetch-failure
  sentinel never mass-prunes.
- Files: `server/notifications/store_test.go`

##### Task 4.4.1c: Add `TestEnforceRetention_GatesOrphanSweep_ByOrphanPruneInterval` (~5 min)
- In `server/notifications/store_test.go`, set a store's `existenceChecker` to a call-counting
  stub, call `Append` twice in immediate succession (well within `orphanPruneInterval`), and
  assert the stub was invoked at most once across both calls. Then advance `lastOrphanPruneAt`
  (directly, or via a test-only setter/clock seam) past `orphanPruneInterval` and call `Append`
  again, asserting the stub is now invoked a second time — proving Task 4.2.1b's gate actually
  decouples the sweep from firing on every single append.
- Files: `server/notifications/store_test.go`

##### Task 4.4.1d: Add `eventToRecord` `SessionScoped` tests (~4 min)
- In `server/notifications/subscriber_test.go` (create if it doesn't exist — check first), add
  positive (`metadata[events.MetadataKeySessionScoped] == "true"`) and negative (key absent)
  cases.
- Files: `server/notifications/subscriber_test.go`

---

### Epic 5: `wireRateLimitCallbacks` Hidden gate + metadata (AC1/AC2, Finding 1)

**Goal**: Close the third unguarded `events.NewNotificationEvent` producer identified during the
Phase 4 pre-mortem (P1 Failure #1): `SessionService.wireRateLimitCallbacks`
(`server/services/session_service.go:3848-3898`) registers two closures —
`onDetected`/`onRecovery` — on **every** `Instance`, via `wireCallbacks` (line 956) → called
unconditionally from `CreateDirectorySession` (line 869), `CreateWorktreeSession` (line 918),
`WireInstanceCallbacks` (line 779, the `Registry.Acquire` onConstruct hook), and one more call site
(line 423) — including `SpawnReviewSession`'s `Hidden=true` review sessions
(`SpawnReviewSession` → `CreateDirectorySession(..., hidden=true)` → `wireCallbacks(instance)` →
`wireRateLimitCallbacks(instance)`, confirmed by reading the actual call chain; there is no
`Hidden` branch anywhere in it). Both closures publish "Session X rate limited"/"...resumed after
rate limit" notifications with a real `sessionID`/`inst.Title`, no `Hidden` check, and a trailing
`nil` metadata argument (no `item_id`, no `session_scoped`) — so a `Hidden` review/re-review
session that hits an API rate limit mid-review produces the exact dead-link Notifications-page
entry this project exists to eliminate, unaffected by Epic 2/3's fixes because this is a
structurally separate producer.

Unlike Epic 1/2's review-queue path, a rate-limit event has no `AttentionReason`/`Determine()`-
derived reason to preserve as a narrowing safety net (Design Decision 1's narrowing only applies to
`Determine()`/`OnItemAdded`'s review-queue-derived reasons) — suppression here is **unconditional
on `Hidden`**, matching Epic 3's generic done/stuck notifier (Design Decision 6), not Epic 1/2's
reason-scoped gate.

#### Story 5.1: Gate `onDetected`/`onRecovery` on `inst.Hidden`, stamp `item_id`/`session_scoped` metadata
**As an** operator, **I want** a `Hidden` session's rate-limit detection/recovery events to never
reach the Notifications page, and a real session's rate-limit notifications to route to "View in
Backlog" when backlog-linked, **so that** this third producer stops reopening the exact dead-link
problem Epic 2/3 close for the other two.
**Acceptance Criteria**:
- Both closures inside `wireRateLimitCallbacks` (`server/services/session_service.go:3855-3897`)
  are wrapped in `if inst.Hidden { return }` as their first statement (`inst` is already captured
  in the closures' lexical scope — no new parameter needed).
- For the non-Hidden case, each closure resolves `linkedItemID` via
  `s.concStorage.GetItemSessionBySessionUUID(ctx, inst.UUID)` — reusing the `concStorage
  *session.Storage` field already on `SessionService` (line 82, "the concrete backing store...nil
  when storage is a fake `InstanceStore`... callers must nil-check", already nil-checked elsewhere
  e.g. line 1204) rather than adding a new query or widening the `session.InstanceStore` interface
  (`s.storage`, line 74, does **not** expose `GetItemSessionBySessionUUID` — only the concrete
  `*session.Storage` does). Guarded by `s.concStorage != nil`, with a bounded
  `context.WithTimeout(context.Background(), rateLimitLookupTimeout)` (new 2s const, matching
  `itemSessionLookupTimeout`'s precedent from Task 2.1.1b) since these closures receive no `ctx`
  parameter from their caller.
- Both closures' trailing `nil` metadata argument to `events.NewNotificationEvent(...)` becomes
  `events.SessionScopedMetadata(nil, linkedItemID)` — the same shared helper Task 2.2.1a/3.2.1b
  use, not a third hand-built map literal.
- A lookup failure (including not-found) is handled silently (empty `linkedItemID`, metadata still
  gets `session_scoped: "true"` via the helper) except a real (non-`ErrNotFound`) error, which is
  logged at `Warn` — mirroring Task 2.1.1b's pattern.
- **Given** `inst := &session.Instance{Title: "review:153f8eac", UUID:
  "aaaa1111-2222-3333-4444-555566667777", Hidden: true}`, **when** `wireRateLimitCallbacks(inst)`'s
  `onDetected` closure fires (simulating a rate-limit detection), **then**
  `s.eventBus.Publish(events.NewNotificationEvent(...))` is never called — but the accompanying
  `s.eventBus.Publish(events.NewSessionUpdatedEvent(...))` call (session state sync, not a
  Notifications-page entry, and out of this AC's scope) still fires unmodified; only the
  `NewNotificationEvent` call is gated.
- **Given** a non-Hidden `Instance{UUID: "bbbb2222-3333-4444-5555-666677778888"}` linked via
  `ItemSessionData{SessionUUID: "bbbb2222-...", SessionRole: "work", ItemID:
  "153f8eac-c454-4fa3-a8f4-83b070b9a035"}`, **when** `onRecovery` fires with `success: true`,
  **then** the published notification's metadata is `{"item_id":
  "153f8eac-c454-4fa3-a8f4-83b070b9a035", "session_scoped": "true"}` (previously `nil`).
**Files**: `server/services/session_service.go`

##### Task 5.1.1a: Add the `Hidden` gate to both closures (~4 min)
- In `server/services/session_service.go`, add `if inst.Hidden { return }` as the first statement
  of both the `onDetected` and `onRecovery` closures inside `wireRateLimitCallbacks` (currently
  lines 3857 and 3875).
- Files: `server/services/session_service.go`

##### Task 5.1.1b: Thread `linkedItemID` + `events.SessionScopedMetadata` into both closures (~6 min)
- Add a new `const rateLimitLookupTimeout = 2 * time.Second` near `wireRateLimitCallbacks`. In
  each closure, after the `Hidden` guard, add:
  ```go
  var linkedItemID string
  if s.concStorage != nil {
      lookupCtx, cancel := context.WithTimeout(context.Background(), rateLimitLookupTimeout)
      itemSession, err := s.concStorage.GetItemSessionBySessionUUID(lookupCtx, inst.UUID)
      cancel()
      if err != nil {
          if !errors.Is(err, session.ErrNotFound) {
              log.Warn("wireRateLimitCallbacks: ItemSession lookup failed", "session", inst.UUID, "err", err)
          }
      } else if itemSession.BacklogItemID != "" {
          linkedItemID = itemSession.BacklogItemID
      }
  }
  ```
  Then change both `events.NewNotificationEvent(...)` calls' trailing `nil` argument to
  `events.SessionScopedMetadata(nil, linkedItemID)`. `"context"`/`"errors"`/`"time"` are already
  imported in this file (Task 2.1.1b's note applies here too, for `errors`).
- Files: `server/services/session_service.go`

#### Story 5.2: Tests for Epic 5
**Acceptance Criteria**:
- A test proving `Hidden: true` suppresses both `onDetected` and `onRecovery`'s
  `NewNotificationEvent` publish, and a positive-metadata test for the non-Hidden,
  backlog-linked case, following this file's existing test harness conventions (search
  `session_service_test.go` for existing `wireCallbacks`/rate-limit test fixtures to match style).
**Files**: `server/services/session_service_test.go`

##### Task 5.2.1a: Add `TestWireRateLimitCallbacks_SuppressesNotification_When_InstanceHidden` + metadata test (~8 min)
- Add `TestWireRateLimitCallbacks_SuppressesNotification_When_InstanceHidden` (covering both
  `onDetected` and `onRecovery` via `t.Run` subtests) and
  `TestWireRateLimitCallbacks_StampsItemIDMetadata_When_BacklogLinkedAndNotHidden` to
  `server/services/session_service_test.go`.
- Files: `server/services/session_service_test.go`

---

## Cross-Epic Verification Checklist (run once all epics land)

- [ ] `make build && make test` passes (per `.claude/rules` build/test conventions).
- [ ] `make lint` passes.
- [ ] All four acceptance criteria's Given-When-Then examples above have a corresponding
      passing test.
- [ ] `docs/registry/` — confirm this change adds no new RPC/UI feature (it's pure backend
      plumbing on existing notification paths) and therefore requires no
      `docs/registry/features/*` update per `.claude/rules/feature-registry.md`.
- [ ] No `session/ent/schema/*` file was touched (Migration Plan's "omitted" claim holds).
- [ ] Epic 5 (`wireRateLimitCallbacks`) lands and its tests pass — this was a Phase 4 pre-mortem
      P1 addition (Finding 1), not part of the original Epic 2/3 scope, and is easy to
      accidentally treat as optional if skimming only the original plan sections.
- [ ] The narrowed Design Decision 1 (`ReasonErrorState`/`ReasonTestsFailing` still notify even
      when `Hidden`) is reflected consistently in Epic 1 (`Determine()`) *and* Epic 2
      (`OnItemAdded`) — both Task 1.1.1b's and Task 2.3.1d's safety-net test cases pass.
