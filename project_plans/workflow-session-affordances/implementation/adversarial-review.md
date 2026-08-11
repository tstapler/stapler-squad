# Adversarial Review: Workflow Session Affordances Plan

**Reviewed:** 2026-06-13
**Plan:** `project_plans/workflow-session-affordances/implementation/plan.md`
**Reviewer:** Adversarial review pass (automated)

---

## Blockers

**BLOCKER:** `maybeAutoArchive` conflicts with `archive_after_hours` — the delay feature is already broken by existing code

The existing `maybeAutoArchive` in `server/services/session_service.go:3774` archives workflow sessions **immediately** when they emit `EventExited`, with no delay. The plan's retention enforcer is designed to archive sessions where `updated_at < now() - archive_after_hours hours AND archived_at IS NULL`. But because `maybeAutoArchive` fires on exit and sets `archived_at = now()` unconditionally, every workflow session will already have a non-NULL `archived_at` before the retention enforcer's hourly sweep runs. The enforcer's time-based archive predicate (`AND archived_at IS NULL`) will never match any completed session — the delay feature is dead on arrival.

To make `archive_after_hours` work, the plan must either:
1. Suppress `maybeAutoArchive` for workflows that have `archive_after_hours > 0` (conditionally wire the callback), or
2. Change `maybeAutoArchive` to only archive if `archive_after_hours == 0` (immediate mode vs delayed mode), or
3. Have the retention enforcer query `archived_at NOT NULL AND archived_at < cutoff` and hard-delete those rows instead (different semantic entirely).

None of these paths are mentioned in the plan. FR-4a (default 24-hour delay) is unimplementable as written.

---

**BLOCKER:** Status constant mismatch between plan and actual DB values

Task 1.3.1 specifies the retention enforcer status guard as:
```go
session.StatusNotIn(SESSION_STATUS_ACTIVE, SESSION_STATUS_CREATING, SESSION_STATUS_PAUSED)
```
using constants named after proto enum values. The `session.StatusNotIn` ent predicate takes `int` values from the DB. The DB stores Go `session.Status` integers: `Creating=0, Active=1, Paused=2, Stopped=3, Hibernated=4`. The proto wire values are completely different: `SESSION_STATUS_ACTIVE=1, SESSION_STATUS_PAUSED=4, SESSION_STATUS_CREATING=6`.

If the implementer interprets `SESSION_STATUS_CREATING=6` and `SESSION_STATUS_PAUSED=4` as the values to pass to `StatusNotIn`, the predicate will exclude `Hibernated=4` (which maps to `SESSION_STATUS_HIBERNATED`, not `SESSION_STATUS_PAUSED`) and **will not exclude `Paused=2`** at all. Active paused sessions will be archived, corrupting live worktrees.

The correct guard must use Go-layer constants: `int(session.Active), int(session.Creating), int(session.Paused)` (values 1, 0, 2). This needs an explicit callout in Task 1.3.1 since the confusion is built into the plan's own language.

---

**BLOCKER:** `WorkflowData` struct does not exist

Task 1.2.2 says: "Update the `WorkflowData` struct (or equivalent) to include `KeepSessions *int` and `ArchiveAfterHours *int`." There is no `WorkflowData` struct in `session/workflow_repository.go`. The actual types are `WorkflowCreateInput` and `WorkflowUpdateInput`. The plan must target these specific structs (adding `KeepSessions *int` and `ArchiveAfterHours *int` as optional fields to both, following the pointer-field pattern of `WorkflowUpdateInput`). Using a phantom struct name means the implementer will search, not find it, and may add a new struct instead of updating the right ones.

---

**BLOCKER:** FR-1d (filter by workflow ID in session list) has no task

Requirements FR-1d: "The session list supports filtering by workflow ID (show only sessions from a specific workflow)." This is listed under FR-1 (Session List — Visual Identity) but no task in any Epic covers it. The server-side `workflow_id` filter already exists in `ListSessionsRequest` (field 7), but there is no frontend UI control to invoke it. The only place this filter is used is `WorkflowsPanel.RecentRuns`. Epic 2 (Session List Frontend) has no story for a workflow-filter dropdown or control in `SessionList`. This requirement is completely dropped.

---

## Concerns

**CONCERN:** `InstanceToProto` signature change is high-blast-radius

`InstanceToProto` in `server/adapters/instance_adapter.go` currently takes `(inst *session.Instance)` with no additional parameters. The plan adds a `workflowNames map[string]string` parameter. There are **13 call sites** across `session_service.go` (lines 748, 774, 804, 834, 841, 858, 1069, 1339, 1395, 1440, 2195, 2252, 2557). Every single caller must be updated, plus `instance_adapter_test.go`. The plan mentions updating "all callers" but the risk register only notes "Update fixture in same PR as adapter change" for tests — it doesn't call out the 13 specific call sites that need changes. A partial update (missing one caller) will produce a compile error, but the task description's vagueness increases the risk of the implementer scoping it incorrectly.

Suggestion: specify explicitly that all 13 callers of `adapters.InstanceToProto` in `session_service.go` must be updated.

---

**CONCERN:** `defaults` mismatch: plan says `0=disabled`, requirements say `default=1` and `default=24`

Requirements FR-4a: "default: 24 hours". FR-4c: "default: keep 1". FR-5a: "`keep_sessions` field (integer, **default: 1**)". FR-5b: "`archive_after_hours` field (integer, **default: 24**)".

The plan's ent schema adds these fields as `Optional().Default(0)` and ADR-1 explicitly says "Zero means disabled." The migration note says existing workflows get nil (not zero). But the requirements are clear that a freshly-created workflow should default to keeping 1 session and archiving after 24 hours. These are **conflicting defaults**. The plan silently drops the 1/24 defaults and uses 0-means-disabled, which means newly-created workflows will never auto-archive and never limit sessions unless the user explicitly sets the values. This violates FR-4a, FR-4c, FR-5a, FR-5b as written.

If the decision is to change defaults to 0=disabled, the requirements document must be updated and the change justified — it currently is not.

---

**CONCERN:** `force_active` flag referenced in ADR-5 but absent from proto definition in Task 1.1.3

ADR-5 says: "Both take `workflow_id` + optional `force_active` flag." Task 1.1.3 defines `ArchiveWorkflowSessionsRequest` with only `string workflow_id = 1` — no `force_active` field. Task 1.4.1 describes the implementation using a status guard to skip active sessions but makes no provision for a bypass flag. Either the ADR is wrong (remove the flag mention) or the proto definition is incomplete (add `bool force_active = 2`). As written, these two sections contradict each other.

---

**CONCERN:** `DeleteWorkflowFailedSessions` heuristic is underspecified and fragile

Task 1.4.2 describes "failed" sessions using two alternative definitions with no decision made:
1. Sessions where `initial_prompt != "" AND last_meaningful_output IS NULL`
2. Sessions created more than 30 minutes ago with no meaningful output

The first requires that workflow sessions always have `initial_prompt` set — but `initial_prompt` is set by the session driver on reaching Ready state, not at creation. A session that never reached Ready will have `initial_prompt == ""`, meaning the `initial_prompt != ""` filter would exclude the most common failure mode (the session never started). The second definition ("created more than 30 minutes ago") is a magic number with no basis in the requirements.

Additionally, Task 1.4.2 says "Archive (not hard-delete)" but the task is named `DeleteWorkflowFailedSessions` and FR-6c says "Delete failed runs." The naming conflict (Delete vs Archive) should be resolved before implementation.

---

**CONCERN:** Bulk archive in Task 1.4.1 iterates live in-memory instances only — misses stopped sessions evicted from memory

Task 1.4.1 says: "List all live instances with `inst.WorkflowID == req.Msg.WorkflowId`." `FindLiveInstance` and `GetInstances` on `reviewQueuePoller` return only sessions held in the in-memory poller. Sessions that are Stopped and have been evicted (if any eviction path exists) or sessions that were never loaded after restart would not appear. The existing `ListSessions` RPC already does this filtering in-memory (line 774), but the research notes that sessions evicted from memory would be missed.

For a correct "Archive all sessions" feature, the bulk archive should query the ent DB directly (like the retention enforcer) rather than iterating the in-memory poller. This mirrors the pattern in the research doc: "For retention/cleanup purposes a direct DB query will be needed."

---

**CONCERN:** `SessionDetailView` CSS import target is wrong

Task 3.1.1 says: "Add CSS to `SessionDetailView.css.ts` (or create if not exists as `.css.ts`)." `SessionDetailView.tsx` actually imports from `"./SessionDetail.css"` (which maps to `SessionDetail.css.ts`), not `SessionDetailView.css.ts`. Both files exist; `SessionDetailView.css.ts` is a different file that contains different styles. Adding `workflowSection` to `SessionDetailView.css.ts` and referencing it as `styles.workflowSection` will fail at import time because `SessionDetailView.tsx` imports from `SessionDetail.css`, not `SessionDetailView.css`. The task must specify `SessionDetail.css.ts`.

---

**CONCERN:** `server.go` doesn't import `workflows` package — plan's Task 1.3.2 code won't compile without adding the import

Task 1.3.2 calls `workflows.StartRetentionEnforcer(serverCtx, entClient, workflowRepo, time.Hour)`. `server.go` does not currently import `"github.com/tstapler/stapler-squad/server/workflows"` — the `workflows.Scheduler` type in `RuntimeDeps` is declared in `dependencies.go` which does import it, but `server.go` itself does not. Adding the call requires adding the import.

Additionally, `entClient` in this call is not defined in scope. The correct expression is `deps.Storage.GetEntClient()` which can return `nil` if the backend is not ent-based — the call needs a nil guard matching the pattern of the analytics enforcer (`if deps.AnalyticsEntClient != nil`).

---

**CONCERN:** `groupSessions()` signature change breaks existing callers and tests — no migration path specified

Adding `options?: { workflowIdToName?: Map<string, string> }` as a third optional parameter changes the TypeScript function signature. There are **5 existing call sites** in `strategies.test.ts` and **1 in `SessionList.tsx`**. TypeScript optional parameters do not break callers that omit them, so this is backward compatible — but the plan says existing callers pass `undefined` or omit it, which is only true if the parameter is truly optional (not required). The plan's Task 2.2.2 signature change to `options?: {...}` is correct TypeScript, but it should be explicitly verified that the current call in `SessionList.tsx` at line 426 (`groupSessions(sortedSessions, groupingStrategy)`) compiles without the third argument.

---

**CONCERN:** `useWorkflows()` added to `SessionList` creates N+1 RPC on every session list render

Task 2.2.3 adds `useWorkflows()` to `SessionList`. This hook calls `ListWorkflows` on mount and periodically. `SessionList` is rendered in the main application layout — it will now fire an additional `ListWorkflows` RPC on page load. The plan notes this as "Acceptable given workflows are small sets" but doesn't account for the fact that `SessionList` can be rendered multiple times (split-pane mode creates multiple `SessionList` instances, each with its own `storageKeyPrefix`). Two `SessionList` instances = two separate `ListWorkflows` pollers running simultaneously. The plan should use a `WorkflowsContext` (similar to how `SessionList` uses contexts for review queue) rather than a per-instance hook to avoid duplicate polling.

---

**CONCERN:** `workflowNameCache` in `SessionService` is not in the `SessionService` struct — undefined home

Task 1.2.3 says "Add `workflowNameCache map[string]string` + `workflowNameMu sync.RWMutex` to `SessionService`." `SessionService` is defined in `server/services/session_service.go`. The plan doesn't show the struct definition or how `workflowRepo`/`workflowRepo` access is wired since `SessionService` currently delegates workflow operations to `workflowSvc *WorkflowService`. `WorkflowService` holds the `WorkflowRepository` reference, not `SessionService`. To populate the name cache, `SessionService` would need either a new dependency on `WorkflowRepository` or a method call to `workflowSvc.ListWorkflows()`. Neither is specified.

---

## Minor Issues

**MINOR:** Plan says retention enforcer mirrors `server/analytics/retention.go` but the analytics enforcer takes completely different parameters

The analytics retention enforcer signature is `analytics.StartRetentionEnforcer(ctx, client, maxRows, maxAgeDays, escapeRetentionDays)` — it takes three `int` configuration values, not a `WorkflowRepository`. The proposed `workflows.StartRetentionEnforcer(ctx, entClient, workflowRepo, interval)` is a different shape. Saying "mirror" the analytics pattern is misleading — the goroutine structure is the same but the parameters and logic differ substantially.

---

**MINOR:** `make generate-proto` instruction in Task 1.1.3 says "run after all proto tasks (1.1.1–1.1.3) are complete" — this is correct but should explicitly note that ent codegen (`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`) must also run after Task 1.2.1 and that both are prerequisites before any Go compilation of the new code.

---

**MINOR:** Proto field numbers in `CreateWorkflowRequest` — the plan adds fields at 12 and 13. `CreateWorkflowRequest.cron_enabled` is field 11 (confirmed from proto source). Fields 12 and 13 are unoccupied. This is correct.

---

**MINOR:** `WorkflowFormData` interface is defined in `web-app/src/lib/hooks/useWorkflows.ts` (not a separate file). Task 4.1.1 says "File: `web-app/src/lib/hooks/useWorkflows.ts`" — this is correct. However the `createWorkflow` call at line 89 of `useWorkflows.ts` uses individual named fields, not spread of `WorkflowFormData`. When adding `keepSessions` and `archiveAfterHours`, both the interface and the individual `create(CreateWorkflowRequestSchema, {...})` call body must be updated.

---

**MINOR:** The plan does not address `WatchSessions` events — when a session is archived by the retention enforcer (a direct DB write), the in-memory `SessionService` will not fire a `SessionArchivedEvent` to `WatchSessions` subscribers. The session will disappear from the UI only on the next poll/reconnect. For immediately-triggered archives (bulk archive RPC), this is fine since the RPC modifies the in-memory instance. But for retention-enforcer archives (direct DB write, no in-memory update), the frontend will show stale data until next reconnect. This is a UX gap not mentioned in the plan's risk register.

---

**MINOR:** `SessionCard.tsx` has no `workflowBadge` CSS class yet. The plan creates it in `SessionCard.css.ts` using `vars.color.accentBg` — that token exists in `theme-contract.css.ts`. The implementation is correct but the plan should specify that the badge goes inside the existing `<div className={badges}>` container, after `autonomousBadge` (position 9 in the badge order per the research). This is the correct location but the plan's code snippet shows it inside the badges row without explicitly stating where in the JSX the badge is inserted.

---

**Verdict: BLOCKED**

Three blockers must be resolved before implementation:
1. The `archive_after_hours` retention feature is logically broken by the existing `maybeAutoArchive` immediate-archive behavior — needs an explicit design decision on how these interact.
2. The status constant mismatch in the retention enforcer would archive actively-paused sessions (corrupting worktrees).
3. `WorkflowData` is a phantom struct name — the actual types are `WorkflowCreateInput` and `WorkflowUpdateInput`.

---

## Round 2 Review (post-patch)

**Reviewed:** 2026-06-13
**Scope:** Verify the 3 original blockers are resolved; check for new issues introduced by the patches.

---

### Blocker 1: `maybeAutoArchive` conflicts with `archive_after_hours` — RESOLVED (design decision present, implementation detail outstanding)

The plan now includes ADR-4 and an explicit guard in `maybeAutoArchive`:

```go
if wf, ok := s.workflowNameCache[inst.WorkflowID]; ok && wf.archiveAfterHours > 0 {
    return
}
```

The logic is correct: if the workflow's cache entry shows `archiveAfterHours > 0`, the immediate archive is skipped and the retention enforcer handles it. The design decision is now on paper.

**Remaining sub-issue (not a new blocker — a pre-existing concern now more concrete):** The guard consults `s.workflowNameCache[inst.WorkflowID]` but `SessionService.workflowSvc` is injected via `SetWorkflowService` after construction (deferred setter). The plan specifies adding `workflowRepo session.WorkflowRepository` as a new constructor parameter to `SessionService`. However, `WorkflowRepo` is already available in `RuntimeDeps` (confirmed: `deps.WorkflowRepo session.WorkflowRepository` at line 384 of `dependencies.go`). The wiring path is unambiguous, so this is implementable. The cache-miss case (`ok == false`) means "cache not yet populated" and the guard will fall through to immediate archive — a race on startup. This is a known risk acknowledged in the plan's risk register ("cache refreshed by background goroutine every minute"). The startup race window is bounded and acceptable.

**Verdict on Blocker 1:** RESOLVED.

---

### Blocker 2: Status constant mismatch — RESOLVED (correct constants now specified)

Task 1.3.1 now includes an explicit callout box:

> "The correct mapping is: Creating=0, Active=1, Paused=2, Stopped=3, Hibernated=4. The proto wire values are different and must NOT be used here. Use the Go ent constants: `session.StatusIn(session.Active, session.Creating, session.Paused)` where these are the typed status constants from the generated ent code, or use the integer literals 0, 1, 2."

**Codebase verification:** `session/instance.go` confirms `Creating=0, Active=1, Paused=2, Stopped=3, Hibernated=4`. `session/ent/session/where.go` confirms `StatusNotIn(vs ...int)` takes raw int values. The plan's guidance to use `int(session.Active)` etc. (Go-layer constants from `session/instance.go`, not ent-generated code) is the correct approach. The parenthetical "or use the integer literals 0, 1, 2" is imprecise — Go ent generated `StatusNotIn` takes `int`, and `session.Active` is of type `session.Status` (a named `int` type defined in `session/instance.go`), so the call will be `int(session.Active)` etc. This is minor; the plan is directionally correct.

**Note:** The plan's Task 1.3.1 body still also says "The enforcer uses ent predicates to exclude sessions where status is Active (1), Creating (6), or Paused (4)" in the Migration/Rollout Notes section (line 499). `Creating=6` and `Paused=4` are the proto wire values, not the DB values. This is a latent inconsistency in the Migration Notes section — it contradicts the correct values given in Task 1.3.1. An implementer reading only the Migration Notes would get the wrong values. This is a **CONCERN**, not a blocker (the authoritative section is correct).

**Verdict on Blocker 2:** RESOLVED — but the Migration/Rollout Notes section still states incorrect status values (Creating=6, Paused=4). Flag for correction before implementation starts.

---

### Blocker 3: `WorkflowData` phantom struct — RESOLVED

Task 1.2.2 now explicitly says:

> "Update `WorkflowCreateInput` and `WorkflowUpdateInput` structs (the actual types — not a phantom `WorkflowData` struct)"

**Codebase verification:** `session/workflow_repository.go` lines 21-38 confirm `WorkflowCreateInput` and `WorkflowUpdateInput` are the actual types. The plan now targets the correct structs.

**Verdict on Blocker 3:** RESOLVED.

---

### New Issues Introduced by the Patches

#### NEW CONCERN: `workflowMeta` type referenced before definition — plan-level inconsistency

ADR-4's guard code snippet uses `s.workflowNameCache[inst.WorkflowID]` and returns a value with a `.archiveAfterHours` field (implying a struct), while ADR-6 later defines `workflowMeta = struct{ name string; archiveAfterHours int }` and renames the field to `workflowMetaCache`. The guard code in ADR-4 still uses `workflowNameCache` (the old name) and accesses `.archiveAfterHours` on a value type whose struct is defined elsewhere. Task 1.2.3 uses the name `workflowMetaCache` consistently. ADR-4 and ADR-6 give the cache two different names (`workflowNameCache` in ADR-4, `workflowMetaCache` in ADR-6). An implementer must reconcile these — the correct name per ADR-6 is `workflowMetaCache`. The ADR-4 pseudocode is stale relative to ADR-6. This is a CONCERN, not a blocker (the intent is clear), but the inconsistency will cause confusion during implementation.

#### NEW CONCERN: `workflowRepo` dependency injection path is underspecified for `SessionService`

ADR-6 says "add `workflowRepo` as a new constructor parameter." The current `NewSessionService(storage session.InstanceStore, eventBus *events.EventBus)` has only 2 parameters. Adding `workflowRepo session.WorkflowRepository` as a third parameter means every call site that constructs `SessionService` must be updated. The plan does not list those call sites. In `server/dependencies.go`, `NewSessionService` is called at line 267 (via `NewSessionServiceWithEntClient` which wraps it). Adding the parameter to the constructor will break `NewSessionServiceWithEntClient` and any direct test-mode construction. The plan should enumerate these construction sites or recommend the same deferred-setter pattern already used for `workflowSvc` (avoiding the constructor change entirely). Treating this as a CONCERN since the implementation is feasible but the plan understates the blast radius.

#### NEW CONCERN: Migration Notes section contradicts Task 1.3.1 status values

As noted above in Blocker 2 resolution: the Migration/Rollout Notes at line 499 state "Creating (6), Paused (4)" — these are proto wire values, not DB values. Task 1.3.1 correctly identifies DB values as Creating=0, Active=1, Paused=2. The contradiction in the same document poses a real risk of the implementer reading the wrong section. Must be corrected before implementation.

#### NEW CONCERN: `SessionService` has no direct access to `entClient` for bulk archive RPCs

Task 1.4.1 shows `s.entClient.Session.Query()` in the bulk archive implementation, but `SessionService` has no `entClient` field (confirmed: the struct at line 57 has no such field). The existing pattern in `session_service.go` accesses the DB through `s.storage` (an `InstanceStore`). To query ent directly, the implementation must either:
1. Cast `s.storage.(*session.Storage).GetEntClient()` (fragile, breaks if storage is not ent-backed), or
2. Add `entClient *ent.Client` as another new dependency to `SessionService`, or
3. Route the bulk archive through `s.workflowSvc` / a new repository method.

The plan does not specify which approach to use. The analytics pattern shows `session_service.go` accepting an ent client via `SetAnalyticsClient` (a setter); the same pattern could be used here (`SetEntClient` or similar). This needs a design decision before implementation.

#### EXISTING CONCERN STILL PRESENT: ADR-5 `force_active` flag contradiction

The original review flagged that ADR-5 mentions `force_active` but Task 1.1.3's proto definition omits it. The patched plan silently drops `force_active` from ADR-5 prose ("Both take `workflow_id` only (no `force_active` flag)") but the ADR text still says "Both take `workflow_id` + optional `force_active` flag" — wait, let me recheck: ADR-5 now reads "Both take `workflow_id` only (no `force_active` flag — active sessions are always skipped)." This is consistent with Task 1.1.3. The contradiction is now resolved in the plan text.

**Verdict: RESOLVED** (ADR-5 was updated to match Task 1.1.3).

#### EXISTING CONCERN STILL PRESENT: `SessionDetailView` CSS import target

Task 3.1.1 now correctly specifies `SessionDetail.css.ts` ("Add CSS to `SessionDetail.css.ts`"). Original concern is resolved.

#### EXISTING CONCERN STILL PRESENT: `server.go` import and nil guard

Task 1.3.2 now includes both the import statement and a nil guard pattern. The plan says to use `deps.EntClient` but the actual field in `RuntimeDeps` is not `EntClient` for the session DB — `EntClient` in `BuildOptions` is a test-only override; `deps.WorkflowRepo` is already available but is the workflow repository, not the raw ent client. The retention enforcer needs either `deps.WorkflowRepo` (for listing workflows) and direct DB access (for updates), or the session DB's ent client. The session ent client is accessed via `storage.GetEntClient()` in `dependencies.go`. In `server.go`, the session DB's ent client is not directly held in `RuntimeDeps` as a named field — it lives inside `deps.Storage.(*session.Storage).GetEntClient()`. This means Task 1.3.2's `deps.EntClient` reference will not compile. The analytics pattern uses `deps.AnalyticsEntClient`, which is an explicitly named field in `RuntimeDeps`. No equivalent field exists for the session DB ent client. This is a **CONCERN** (the plan's code will not compile as written); the implementer needs to add a session ent client field to `RuntimeDeps` or derive it from storage.

---

### Summary

| Original Blocker | Status |
|---|---|
| Blocker 1: `maybeAutoArchive` / `archive_after_hours` conflict | RESOLVED |
| Blocker 2: Status constant mismatch (wrong proto vs. DB values) | RESOLVED (residual: Migration Notes section still has wrong values) |
| Blocker 3: `WorkflowData` phantom struct | RESOLVED |

| New Issues | Severity |
|---|---|
| ADR-4 / ADR-6 cache name inconsistency (`workflowNameCache` vs `workflowMetaCache`) | CONCERN |
| `workflowRepo` constructor injection blast radius unspecified | CONCERN |
| Migration Notes section still states incorrect status values (Creating=6, Paused=4) | CONCERN |
| `SessionService` has no `entClient` field — `s.entClient.Session.Query()` in Task 1.4.1 won't compile | CONCERN |
| `deps.EntClient` in Task 1.3.2 does not correspond to a named field in `RuntimeDeps` | CONCERN |

No new blockers were introduced. All 3 original blockers are resolved. However, 5 concerns remain — two of which (the `s.entClient` and `deps.EntClient` issues) will cause compile failures if not addressed before the implementer starts writing code.

**Round 2 Verdict: CONCERNS**
