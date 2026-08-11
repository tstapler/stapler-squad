# Research: Technology Stack & Existing Wiring Patterns

**Agent**: Research Agent 1 (STACK)
**Date**: 2026-07-17
**Scope**: Document the exact stack + existing wiring this feature must reuse, with `file:line` references.

All paths below are repo-relative to the worktree root
`/home/tstapler/Programming/stapler-squad/.claude/worktrees/agent-a17f6a3c8f4ac9297/`.

---

## 0. Framework / Version Baseline (only what's relevant)

From `go.mod`:
- `entgo.io/ent v0.14.5` — ORM (schema-as-code, code-generated client)
- `connectrpc.com/connect v1.19.0` — RPC framework (also legacy `github.com/bufbuild/connect-go v1.10.0` still present)
- `google.golang.org/protobuf v1.36.11`, `github.com/bufbuild/buf v1.57.2` — proto toolchain (`make proto-gen`)

From `web-app/package.json`:
- `react ^19.0.0`, `typescript ^5.9.3`
- `@connectrpc/connect ^2.1.1`, `@bufbuild/protobuf ^2.11.0` — generated TS bindings live in `web-app/src/gen/session/v1/`
- `@vanilla-extract/css ^1.20.1` + `@vanilla-extract/recipes ^0.5.7` — CSS (per ADR-009; new component styles in `.css.ts`)

---

## 1. The Established Opt-In Per-Item Bool-Flag Pattern (trace of `AutoSpawnSession`)

`AutoSpawnSession` (commit `b28ace2f`) is the canonical, most-recent reference. It is a plain
proto3 `bool` (NOT `optional`), written unconditionally on every update. Contrast: `PipelineMode`
(the out-of-scope Epic 1.4 field) is an `optional string` with presence-gating — do NOT copy that
presence pattern; copy the `AutoSpawnSession` unconditional-bool pattern.

The new mergeability-policy flag must touch **all 8 layers** below.

### Layer 1 — ent schema field
`session/ent/schema/backlog_item.go:43-45`
```go
field.Bool("auto_spawn_session").
    Default(false).
    Comment("When true, a work session is spawned automatically once the item reaches ready — no manual 'Spawn Session' click required."),
```
Siblings in the same `Fields()` block: `skip_review_gate` (:39), `skip_planning` (:41),
`pipeline_mode` (:46, the `optional`-style outlier). Add the new `field.Bool(...).Default(false)`
here alongside `auto_spawn_session`.

### Layer 2 — ent regeneration command
`session/ent/generate.go` (the `//go:generate` directive):
```
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
```
Run from `session/ent/` (or `go generate ./session/ent/`). The `--feature sql/upsert` flag is
mandatory (per `.claude/rules/ent-schema-generation.md`) — omitting it silently breaks `UpsertRule`.
Commit all regenerated `session/ent/` files together.

### Layer 3 — proto field (allocate next field number, plain `bool`)
`proto/session/v1/backlog.proto` — the flag appears in **three** messages; `auto_spawn_session`
took the next free number in each:
- `BacklogItem` message: `bool auto_spawn_session = 24;` (:119). Next free = **26** (25 is `optional string pipeline_mode`).
- `CreateBacklogItemRequest`: `bool auto_spawn_session = 10;` (:187). Next free = **12** (11 = pipeline_mode).
- `UpdateBacklogItemRequest`: `bool auto_spawn_session = 12;` (:227). Next free = **14** (13 = pipeline_mode).

Run `make proto-gen` after editing → regenerates `session/gen/session/v1/*.go` and
`web-app/src/gen/session/v1/*_pb.ts`.

### Layer 4 — Go domain structs
`session/repository.go`:
- `BacklogItemData.AutoSpawnSession bool` at **:352** (the struct starts at :342). Add the new bool here.
- `BacklogItemUpdate.AutoSpawnSession *bool` at **:440** (pointer for partial-update presence; struct at :432).

### Layer 5 — repository Create/Update mapping (ent ↔ domain)
`session/ent_repository_backlog.go`:
- **ent → domain** (`backlogItemToData`): `AutoSpawnSession: item.AutoSpawnSession,` at **:145**.
- **domain → ent Create** (`CreateBacklogItem`): `.SetAutoSpawnSession(data.AutoSpawnSession)` at **:210**.
- **domain → ent Update** (`UpdateBacklogItem`): pointer-nil-check pattern at **:443-445**:
  ```go
  if update.AutoSpawnSession != nil {
      u.SetAutoSpawnSession(*update.AutoSpawnSession)
  }
  ```
Add the new field at all three sites.

### Layer 6 — RPC handler read/write (proto ↔ domain)
`server/services/backlog_service.go`:
- **domain → proto** (`backlogItemToProto`, starts :491): `AutoSpawnSession: item.AutoSpawnSession,` at **:501**.

`server/services/backlog_service_lifecycle.go`:
- **Create handler** (`CreateBacklogItem`): `AutoSpawnSession: req.Msg.AutoSpawnSession,` at **:160**.
- **Update handler** (`UpdateBacklogItem`): the unconditional-wrap pattern at **:236-237**:
  ```go
  autoSpawn := req.Msg.AutoSpawnSession
  update.AutoSpawnSession = &autoSpawn
  ```
  NOTE the deliberate contrast documented at :238-245: `PipelineMode` is presence-gated
  (`if req.Msg.PipelineMode != nil`), the bools are NOT. Copy the **bool** (unconditional) style.

### Layer 7 — Frontend form
`web-app/src/components/backlog/BacklogItemForm.tsx`:
- state hook: `const [autoSpawnSession, setAutoSpawnSession] = useState(initialValues?.autoSpawnSession ?? false);` **:72**
- included in submit payload object **:207**, and in the `useCallback` dep array **:216**
- checkbox JSX block **:398-414** (id `backlog-auto-spawn-session`, `data-testid="backlog-auto-spawn-session-checkbox"`, hint text). Clone this block for the new flag.

### Layer 8 — The proto3-bool-reset guard (`currentFlags()` helper) — CRITICAL
`web-app/src/components/backlog/BacklogItemDetail.tsx`:
- `currentFlags` `useCallback` at **:306-311**:
  ```ts
  const currentFlags = useCallback(() => ({
      skipPlanning: item?.skipPlanning ?? false,
      skipReviewGate: item?.skipReviewGate ?? false,
      autoSpawnSession: item?.autoSpawnSession ?? false,
  }), [item]);
  ```
- Spread `...currentFlags()` into **every** partial `updateBacklogItem()` call: **:319** (save notes),
  **:386** (apply AC), **:406** (undo AC), **:440** (gate-reopen feedback). Comment at :302-305 explains
  why: the backend writes these plain-`bool` fields unconditionally on every update, so any partial
  update omitting them silently resets them to `false`. **The new flag MUST be added to `currentFlags()`**
  or it will be zeroed by unrelated saves — this exact bug bit `AutoSpawnSession` (task doc bucket [2],
  commit `b28ace2f`).

### Registry / test obligations
Per `.claude/rules/feature-registry.md` + `.claude/rules/e2e-test-conventions.md`: add/update a
per-feature JSON under `docs/registry/features/backend/`, add a Playwright spec in `tests/e2e/`
(feature-annotation header, `data-testid` locators, no `waitForTimeout`), run `make registry-generate`.
Go unit tests for the toggle already have a template: `server/services/backlog_service_test.go:1922-1985`
(`TestTriggerTriage_AutoSpawnSession_SpawnsWorkSessionWithoutManualClick` + the `...False_...` default guard).

---

## 2. Mergeability + CI Signal Surface (how the code learns "CI green" + "no conflict")

**There is no `backlog_plugin_github*.go` file.** The GitHub signal surface is `gh` CLI shell-outs
wrapped in `session/git/worktree_git.go` on the `GitWorktree` type. `server/services/github_service.go`
is a thin RPC wrapper over the same data.

### `PRStatus` struct — the single machine-checkable signal bundle
`session/git/worktree_git.go:329-376`. Exported fields the policy can read:
- `CIFailing bool` (:332) — true if any check has terminal failure
- `HasBlockingReviews bool` (:334) — a reviewer requested changes
- `HasConflicts bool` (:339) — **the merge-conflict signal**; true when `mergeStateStatus == "DIRTY"` OR `mergeable == "CONFLICTING"`
- `IsClosed bool` (:344), `IsDraft bool` (:349)
- `Mergeable string` (:356) — raw upper-cased GitHub `mergeable` ("MERGEABLE" / "CONFLICTING" / "UNKNOWN")
- `ApprovedCount int` (:357), `ChangesRequestedCount int` (:359)
- `FeedbackText string` (:365) — rendered human summary for the fix agent (conflict-first ordering, `render()` :381)

### `GetPRStatus(prNumber int) (*PRStatus, error)`
`session/git/worktree_git.go:437-554`. Single `gh pr view` call with
`--json statusCheckRollup,reviews,comments,mergeable,mergeStateStatus,state,isDraft` (:445). Evaluation:
- Conflict detection **:504-509**: `mss == "DIRTY" || mg == "CONFLICTING"` → `HasConflicts = true`.
  Both fields are checked deliberately — cli/cli#9583 documents `gh`'s `mergeable` returning stale data
  vs `mergeStateStatus` (comment :498-503).
- CI evaluation **:512-533**: `conclusion FAILURE|TIMED_OUT|CANCELLED` or `state FAILURE|ERROR` → `CIFailing = true`.
- Reviews **:535-545**, comments **:547-550**.

### "CI green AND no conflict" — the exact terminal predicate already in code
`session/backlog_lifecycle.go:1584`:
```go
if !prStatus.CIFailing && !prStatus.HasBlockingReviews && !prStatus.HasConflicts {
    // PR is open and healthy — wait for merge
```
And `prReadyToMergeSolo(info)` in `session/stuck_decisions.go:94`:
```go
return strings.EqualFold(info.Mergeable, "MERGEABLE")
```
(:68 comment notes it deliberately DROPS the `ApprovedCount > 0` requirement — a permanent
false-negative on a single-user self-authored PR). This solo-ready predicate is the closest existing
definition of "genuinely mergeable" and is what already drives the `pr_ready_unmerged` stuck flag.

### Merge-enable + merged-check
- `EnablePRAutoMerge(prNumber int)` — `session/git/worktree_git.go:559-572`: `gh pr merge <n> --auto --squash`.
  Best-effort; fails when repo lacks auto-merge/branch-protection. Already called after PR creation at
  `session/backlog_lifecycle.go:1475`.
- `IsPRMerged(prNumber int)` — `session/git/worktree_git.go:575-588`: `gh pr view --json state --jq .state == "MERGED"`.

---

## 3. Notification Infrastructure

Notifications are **domain events published to an `EventBus`**, then fanned out (streamed to the SPA
via `event_converter.go` → `NotificationEvent` proto; web-push via `PushService`). Two shapes exist:

### 3a. `pkg/events` — `NewNotificationEvent(...)` (used inside `server/services`)
`pkg/events/types.go` — the `Event` struct carries notification fields at **:62-68**:
`NotificationID string`, `NotificationType int32` (maps to `sessionv1.NotificationType`),
`NotificationPriority int32`, `NotificationTitle`, `NotificationMessage`, `NotificationMetadata map[string]string`.
Constructor call signature (see call sites): `NewNotificationEvent(sessionID, "", uuid, type, priority, title, body, metadataMap)`.

Existing operator-notification helpers to mirror (all in `server/services/backlog_service_triage.go`):
- `notifyReworkCapHit(ctx, itemID, itemTitle, currentStatus, capContext)` — **:75-99**. The gold-standard
  pattern: writes a **durable** `BacklogStuckState` row via `s.storage.MarkStuck(...)` + `MarkStuckNotified(...)`
  (restart-surviving, DB-backed notify-once) THEN publishes a `NOTIFICATION_TYPE_WARNING` /
  `PRIORITY_MEDIUM` event via `s.eventBus.Publish(events.NewNotificationEvent(...))` (:91-98). Durable write
  is additive, never gates the notification.
- `notifyTriagePersistFailure(ctx, itemID, itemTitle, failures, statusAdvanced)` — **:106-122**. Simpler:
  eventBus publish only, no durable row.

Both no-op when `s.eventBus == nil`. Enum values used: `sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING`,
`sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM`.

### 3b. `session.Notifier` interface + `l.notify(...)` (used inside the `session` package)
The `session` package cannot import `pkg/events` (import cycle), so it depends on a narrow
`session.Notifier` interface, adapted by `EventBusNotifier` in `server/services/backlog_notifier.go:12-27`
(wraps `server/events.EventBus`). `session/backlog_lifecycle.go` calls `l.notify(itemID, title, body, typeInt, prioInt)`
— e.g. the auto-merge-not-enabled warning at **:1477-1482** (`8`=WARNING, `2`=PRIORITY_MEDIUM as raw ints).

> **Note for planning**: there are two `events` packages — `pkg/events` (imported by
> `server/services/backlog_service_triage.go`) and `server/events` (imported by `backlog_notifier.go`).
> Where the new "ready to merge" notification fires determines which path to use: a `BacklogService`
> method uses `s.eventBus`/`pkg/events`; a `BacklogLifecycleListener` (reconciler) method uses
> `l.notify`/`session.Notifier`. The reconciler already emits the `pr_ready_unmerged` notification via
> the `l.markPRReadyUnmerged` → `MarkStuck`/notify path (`session/backlog_lifecycle.go:1637-1660`), which
> is the most likely home for Behavior 3.

### 3c. Web push (terminal delivery)
`server/services/push_service.go` — `PushNotification` struct :28-36, `SendNotification` :197,
`sendToSubscription` :225 (uses `github.com/SherClockHolmes/webpush-go`, VAPID keys). This is downstream
of the EventBus; the policy feature does not touch it directly.

### 3d. Existing "ready to merge" signal (reuse target for Behavior 3)
`markPRReadyUnmerged` — `session/backlog_lifecycle.go:1637-1660`. Writes/refreshes a durable
`StuckReasonPRReadyUnmerged` row (`domain.StuckReasonPRReadyUnmerged`, `session/domain/backlog.go:38`)
and notifies **once** past `prReadyThreshold` via DB-backed `notified_at` dedup, message
`"... PR #N is green, mergeable, and has been ready to merge for over <threshold>. Merge it on GitHub."`
(:1659). This is the existing "genuine mergeability" surface — Behavior 3 should extend/reuse it rather
than invent a parallel notifier.

---

## 4. Reconciler Tick Machinery

### Scheduler / entry point
`server/dependencies.go:867-884` — a single background goroutine with a **60-second** `time.NewTicker`,
panic-recovered, calling `backlogLifecycleListener.ReconcileStuck(ctx)` each tick. Gated on the
`backlog` feature flag; a one-time `BackfillStuckStates` runs before the ticker starts (:863-865).
This is described in-code as "the only fallback for review-gate respawn, stale-item detection, and
PR-pending polling (merge/CI/conflict)".

### `ReconcileStuck` — the umbrella tick body
`session/backlog_lifecycle.go:806` (calls `ReconcileStuckItems` at :814, then a series of
panic-isolated detectors via `runStuckDetector`):
- `stale_work` (:875), `abandoned_review` (:885), `bouncing` (:892), `orphaned_triage` (:900),
  `self_heal` (:906)
- **`pr_ready+merge_detection` (:912-914)** → `l.ReconcilePRPending(ctx, er)`

### `ReconcilePRPending` — the PR-lifecycle poller (Behavior 2's home)
`session/backlog_lifecycle.go:1508-1630`. Per `pr_pending` item (found via `FindPRPendingItems`, which
filters `PrNumberGT(0)`):
1. `IsPRMerged` → transition to `done` (:1524-1542).
2. `GetPRStatus` (:1545) → the signal bundle from §2.
3. Closed-without-merge branch (:1558-1582): clears cached PR fields, calls `AutoReopenForPRFix`.
4. Healthy branch (:1584-1610): `!CIFailing && !HasBlockingReviews && !HasConflicts` →
   `prReadyToMergeSolo` gate → `markPRReadyUnmerged` (the Behavior-3 notification) else resolve.
5. Unhealthy branch (:1618-1628): CI failure / blocking review / conflict → `AutoReopenForPRFix(ctx, itemID, fixCtx)`
   where `fixCtx` embeds `prStatus.FeedbackText`.

### `AutoReopenForPRFix` — the fix-loop spawner
`server/services/backlog_service_triage.go:568-622+`. Guards:
- item must be `pr_pending` (:577)
- `tombstoneOrphanWorkSessions` then `hasActiveWorkSession` early-return (:597-601) — the churn fix
  (commit `f8f788ab`) that prevents re-transitioning while a fix is in flight
- **rework cap**: `workCount >= maxAutoReworkIterations (3)` → `notifyReworkCapHit`, leave in `pr_pending`
  (:609-613). **This is the fix-loop terminal state** (requirements Open Question 4).
- else transition `pr_pending → in_progress` with optimistic-lock precondition (:615-621) and spawn.

`AutoReopenForPRFix` is invoked through the `PRFixSpawner` interface (`l.getPRFixSpawner()`); it's
overridable in tests. The reconciler calls it; nothing else drives the loop.

### Key crossref for Behavior 1 (auto-create PR on Complete)
PR creation today happens in `pushAndCreatePR` (`session/backlog_lifecycle.go`, `CreatePR` at :1452,
`EnablePRAutoMerge` at :1475, transition to `pr_pending` at :1487-1493). This is the review-gate `onPass`
path. The manual Review-Queue path is `SessionService.RunOneShot` (`server/services/session_service.go:3405`).
Requirements Open Question 2 asks which is the correct integration point for Behavior 1 — the two diverge
(backlog path auto-enables merge; Review-Queue path is fully manual). This is a planning decision, flagged
here as the key structural fork.

---

## 5. Constants & Tuning Knobs Relevant to Risk Controls

`server/services/backlog_service_triage.go`:
- `maxAutoReworkIterations = 3` (:136) — fix-loop / rework cap ceiling
- `maxConcurrentBacklogWorkItems = 2` (:147) — global concurrency cap (spawns beyond it rejected with `CodeResourceExhausted`; added after a 2026-07-12 OOM)
- `defaultTriageCleanupTimeout = 10 * time.Second` (:153), `maxTriageSessionAge = 2 * time.Hour` (:158)

These are `const` today (task doc bucket [3] flags them as "operational tuning knobs" that are not yet
configurable). The requirements' global kill-switch question (Open Question 3) has no existing config
field — `cfg.GetFeatureFlag("backlog")` (`server/dependencies.go:863`) is the closest precedent for a
feature-flag-style gate, and the reconcile ticker itself is already gated on it.
