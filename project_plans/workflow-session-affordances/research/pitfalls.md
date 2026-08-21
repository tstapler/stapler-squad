# Workflow Session Affordances — Research: Pitfalls & Edge Cases

Researched: 2026-06-13
Codebase version: main @ 1a147e0d

---

## 1. Running-session vs Retention Race Conditions

### Current auto-archive mechanism

`maybeAutoArchive` (`server/services/session_service.go:3774`) fires on `EventExited` — the lifecycle event that signals the process terminated. It uses a CAS on `ArchivedAt`:

```go
func (i *Instance) SetArchivedAtIfNil(t time.Time) bool {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    if i.ArchivedAt != nil { return false }
    i.ArchivedAt = &t
    return true
}
```

This prevents double-archive from two concurrent `EventExited` fires (safe already).

### Risk: auto-archive fires while session is still Active

The current `autoArchiveListener.OnLifecycleEvent` only fires on `EventExited` so it cannot fire while `Status == Active`. However, the proposed **"archive after N hours"** retention sweep is a time-based background job, not event-triggered. A background sweeper checking `updated_at + archive_after_hours < now` would not see the status — it would query the DB. Between the DB read and the write, a session could transition from `Stopped → Active` via the resume flow.

**Concretely:** A workflow spawns a session. Session exits (Stopped). A user resumes it (Active) before the retention sweep runs. The sweep archives an Active session.

**Guard recommendation:**
- The retention sweeper must filter `WHERE archived_at IS NULL AND status NOT IN (Creating, Active, Paused)` — exclude non-terminal states.
- In Go: check `inst.IsActive() || inst.IsCreating() || inst.IsPaused()` before setting `ArchivedAt`.
- `IsPaused()` is especially important: a paused session has a live git worktree — archiving it without unpausing first would orphan the worktree (same danger as the existing Hibernated state).

### Risk: "keep N most recent" races with new sessions firing concurrently

If the scheduler fires a new session mid-sweep while counting existing sessions, the count can be off by one, causing an extra deletion or an under-deletion. The sweep should hold an exclusive lock (mutex or DB transaction) while counting + selecting victims:

```
BEGIN;
SELECT id FROM sessions WHERE workflow_id = $1 AND archived_at IS NULL
    AND status IN (Stopped) ORDER BY created_at DESC OFFSET $keep_sessions;
UPDATE sessions SET archived_at = now() WHERE id IN (...);
COMMIT;
```

Without atomicity, concurrent cron fires can each independently conclude "I have N sessions, archive the oldest" and over-archive.

### Status enum values that matter

From `proto/session/v1/types.proto`:
- `SESSION_STATUS_ACTIVE = 1` — live process, never archive
- `SESSION_STATUS_PAUSED = 4` — worktree exists, never archive
- `SESSION_STATUS_CREATING = 6` — mid-creation, never archive
- `SESSION_STATUS_STOPPED = 7` — safe to archive
- `SESSION_STATUS_HIBERNATED = 8` — may be safe to archive (no live process), but verify worktree state first

---

## 2. Zero-Value Proto Field Risks (keep_sessions, archive_after_hours)

### The proto3 zero-value problem

Proto3 does not distinguish "not provided" from "set to zero" for scalar fields. If `WorkflowProto` gets:

```protobuf
int32 keep_sessions = N;
int32 archive_after_hours = M;
```

Then an existing workflow that was saved before these fields were added will have `keep_sessions = 0` and `archive_after_hours = 0` when deserialized — because proto3 fills missing fields with zero-value.

**Dangerous semantics:**
- `keep_sessions = 0` could mean "keep none" → archive everything immediately → catastrophic for existing workflows.
- `archive_after_hours = 0` could mean "archive instantly" → same catastrophe.

### Pattern established in this codebase

The codebase already uses proto `optional` for nullable scalars, e.g. `optional string workflow_id = 7` in `ListSessionsRequest`. The `UpdateWorkflowRequest` uses `optional` throughout so only provided fields are updated:

```protobuf
optional bool cron_enabled = 11;
```

**Recommendation:**
- Declare both fields `optional` in proto so the generated Go field is `*int32`, not `int32`.
- In the ent schema, add as `Optional()` fields (see existing pattern: `field.Bool("cron_enabled").Default(false)` and `field.String("session_type").Optional().Default("directory")`).
- Add explicit defaults: `keep_sessions` default = 0 means "disabled / keep all"; `archive_after_hours` default = 0 means "disabled".
- The sweeper guard: `if wf.KeepSessions == nil || *wf.KeepSessions == 0 { skip }`.
- Document: zero = "off" not "archive everything."

### Migration concern

The ent migration for adding new optional columns to the `workflows` table is additive (SQLite `ALTER TABLE ADD COLUMN` with nullable defaults). See existing pattern: the recent `approval_rule` fix (commit `3f06c420`) made JSON fields Optional to fix SQLite migration. New `int32 Optional().Default(0)` columns are safe.

However the proto `WorkflowProto` returned by `ListWorkflows` will have these new fields at zero for all existing workflows until they are updated. Any frontend code checking `wf.keepSessions > 0` to decide whether to show a retention badge must not confuse zero with "configured to keep 0."

---

## 3. "Suppress by Default" Filter State Migration Concerns

### How filters are currently persisted

`SessionList.tsx` persists all filter state to `localStorage` keyed by prefixed storage keys. The active keys (from `BASE_STORAGE_KEYS`):

```
stapler-squad-search-query
stapler-squad-selected-status
stapler-squad-selected-category
stapler-squad-selected-tag
stapler-squad-hide-paused
stapler-squad-filter-needs-approval
stapler-squad-grouping-strategy
stapler-squad-sort-field
stapler-squad-sort-dir
stapler-squad-visible-columns
```

Each is loaded via `useState(() => loadFromStorage(key, defaultValue))`. If the key is missing from localStorage, the `defaultValue` is used.

### The hidden/archived precedent

Workflow sessions with `workflow_id != ""` currently appear in the default session list because `ListSessions` does not filter by `workflow_id` unless explicitly requested. The "suppress by default" feature would need to either:

**Option A — Server-side filter:** Add `exclude_workflow_sessions: bool` to `ListSessionsRequest` and default it to `true` on the main page call. This is clean but changes the API contract — clients not sending the new field get the old behavior (no suppression), which is backward compatible.

**Option B — Client-side filter:** Add a `hideWorkflow` boolean to `SessionList` state with a localStorage key. Default to `true`. The concern: **existing users** with localStorage state from before this feature would have no `stapler-squad-hide-workflow` key → `loadFromStorage` returns `defaultValue = true` → sessions disappear on upgrade. This is the intended behavior but could feel surprising.

**Breakage risk:** Any component that calls `listSessions` without the new filter but used to see workflow sessions will now miss them. The workflow page's `listSessionsByWorkflow` passes `workflowId` filter explicitly so it is unaffected. But generic session count badges, the review queue, or notification systems that iterate all sessions could silently miss workflow sessions.

**Specific impact on review queue:** `ReviewQueuePoller` at `session/review_queue_poller.go` operates on the in-memory instance list, not the `ListSessions` RPC, so it will still process workflow sessions for approval prompts regardless of the UI filter. This is correct behavior but the UX could confuse users: an approval notification fires for a session they believe is hidden.

**Recommendation:**
- Implement as server-side `include_workflow_sessions bool` defaulting to `false` — opt-in, not opt-out.
- Add a toggle in the filter bar (like `hidePaused`) so users can reveal workflow sessions.
- Add a localStorage key `stapler-squad-show-workflow-sessions` defaulting to `false`.
- The workflow page's `RecentRuns` already passes `workflowId` filter so it is isolated from this change.

---

## 4. Existing Tests That May Break

### Frontend grouping strategy tests

File: `/home/tstapler/Programming/stapler-squad/web-app/src/lib/grouping/strategies.test.ts`

The test at line 52 ("should return single group for Strategy.None") and line 27 ("should group by category") use a hardcoded `mockSessions` array with no `workflow_id` field. If "suppress workflow sessions" is applied inside `groupSessions()` rather than upstream in the filter, these tests would pass vacuously (no workflow sessions in test data). But if the test data is later updated with workflow sessions to test the new strategy, the test must explicitly set `workflowId` on each mock session.

### Grouping strategy cycle test (implicit)

`cycleGroupingStrategy` iterates `Object.values(GroupingStrategy)`. Adding `GroupingStrategy.Workflow` to the enum automatically inserts it into the cycle. The strategies.test.ts has no test for `cycleGroupingStrategy` itself. Any test that asserts the full set of strategies (e.g., count of dropdown options) in the component would fail when a new strategy is added.

File: `/home/tstapler/Programming/stapler-squad/web-app/src/components/sessions/SessionList.tsx` at line 726 renders `Object.entries(GroupingStrategyLabels)` — this will automatically include the new strategy. No test directly asserts on the dropdown option count, but snapshot tests would catch this if any exist.

### Server adapter test

File: `/home/tstapler/Programming/stapler-squad/server/adapters/instance_adapter_test.go`

The adapter test likely asserts on the proto fields of a session. Adding `workflow_badge` or new metadata to `InstanceToProto` would require updating the assertion fixture. Verify: `grep -n "WorkflowId\|workflow" server/adapters/instance_adapter_test.go`.

### Workflow service test

File: `/home/tstapler/Programming/stapler-squad/server/services/workflow_service_test.go`

If `WorkflowProto` gains new fields (`keep_sessions`, `archive_after_hours`), the `entWorkflowToProto` function and its tests must be updated to include these fields. Tests that assert proto equality would fail with unexpected nil/zero values for the new fields.

### Workflow scheduler test (indirect)

File: `/home/tstapler/Programming/stapler-squad/server/workflows/scheduler.go` — `FireNow` creates sessions with `WorkflowId` set. If session creation is modified to consult `keep_sessions` immediately at creation time (to pre-archive old sessions), any integration test calling `FireNow` would now also trigger archive side effects.

### Control mode refcount test

File: `/home/tstapler/Programming/stapler-squad/session/tmux/control_mode_refcount_test.go`

No workflow-related logic found here. This test is unlikely to be affected.

### ImportRulesModal test

File: `/home/tstapler/Programming/stapler-squad/web-app/src/components/sessions/ImportRulesModal.test.tsx`

Tests approval rule import, no session list or workflow dependency. Unaffected.

---

## 5. Bulk Archive Race Condition

### Current ArchiveSession implementation

`ArchiveSession` (`session_service.go:3732`) has no guard against archiving active sessions:

```go
func (s *SessionService) ArchiveSession(...) {
    inst := s.FindLiveInstance(req.Msg.SessionId)
    now := time.Now()
    inst.ArchivedAt = &now          // no status check
    s.storage.SaveInstances([]*session.Instance{inst})
}
```

There is no status check. A running session can be archived via the RPC today.

### The bulk archive race

"Archive all sessions" for a workflow page would loop over the current `listSessionsByWorkflow` result and call `ArchiveSession` for each. While this loop executes, the workflow cron scheduler could fire and create new sessions (from `addCronEntry` closure in `scheduler.go`). The new sessions would not be in the originally fetched list and would survive the bulk archive.

This is acceptable behavior (archive what existed at the moment of the click, not future sessions), but the UX copy must set that expectation: "Archive all current sessions."

More dangerous: if a session is `Active` (the AI is currently working), bulk-archiving it does not stop the process. The session is archived but continues running in tmux. The AI can still write files, make commits, etc. The session is hidden from the list but the process is live. This is the same gap that exists for the single-session `ArchiveSession` RPC.

**Recommendation:**
- For bulk archive (and ideally for single archive), add a guard: refuse to archive sessions in `Active`, `Creating`, or `Paused` states unless a `force: bool` flag is provided.
- Or: stop the session before archiving — but stopping adds latency and may interrupt useful work.
- At minimum, document this behavior and surface a warning in the UI when archiving active sessions.

### No transaction boundary across multiple ArchiveSession calls

`SaveInstances` calls `repo.Update` per-session in a loop with no wrapping transaction. If the bulk archive fails mid-loop (network error, DB error), some sessions will be archived and others not. This is probably acceptable (partial progress is better than all-or-nothing), but the UI should reflect which sessions were successfully archived.

---

## 6. Non-Workflow Sessions in "Group by Workflow" View

### The fallback bucket problem

`groupSessions()` (`strategies.ts:56`) handles missing group keys with a fallback string, e.g. `session.category || "Uncategorized"`. For a Workflow grouping strategy, the equivalent would be:

```typescript
case GroupingStrategy.Workflow:
    groupKeys = [session.workflowId || "No Workflow"];
    break;
```

This creates an "orphan bucket" named "No Workflow" containing all non-workflow sessions. Depending on the user's install, this bucket could be very large (all existing sessions from before workflows were introduced) or empty (if they only use workflows).

**UX risks:**
- The "No Workflow" bucket is not sortable/filterable in a meaningful way — it's just "everything else."
- Users who never use workflows would see their entire session list in a single "No Workflow" bucket, making the Workflow grouping useless noise.
- The `cycleGroupingStrategy` keyboard shortcut (`G`) would cycle through all strategies including Workflow. Users hitting `G` repeatedly could land on Workflow grouping unexpectedly.

**Recommendation:**
- Only show the Workflow strategy option in the grouping dropdown when the user has at least one workflow defined. This avoids presenting a useless grouping.
- Name the fallback bucket "Manual Sessions" rather than "No Workflow" to be less confusing.
- Consider omitting `GroupingStrategy.Workflow` from the `cycleGroupingStrategy` rotation if the user has no workflows (same pattern could apply to `GroupingStrategy.Project` if no projects exist).

### Special groups ordering

The `specialGroups` array in `strategies.ts:152` determines sort order — special groups (Uncategorized, Untagged, etc.) always appear at the end. "No Workflow" / "Manual Sessions" must be added to this array so it sorts last, after all named workflow groups.

---

## 7. Additional Risks Found During Investigation

### WorkflowProto proto field number availability

`WorkflowProto` currently uses fields 1–14. The new `keep_sessions` and `archive_after_hours` fields need field numbers 15 and 16. Field numbers 1–15 are in the "1-byte varint" range (most compact). This is fine — no performance concern. However, `CreateWorkflowRequest` and `UpdateWorkflowRequest` also need corresponding fields. `CreateWorkflowRequest` uses 1–11; `UpdateWorkflowRequest` uses 1–11 (all optional). Adding 12 and 13 (or next available numbers) is straightforward.

### WorkflowID is a plain string, not validated as UUID

In `session/instance.go:262`, `WorkflowID` is a plain `string`. In `server/services/session_service.go:770`, filtering uses string equality: `inst.WorkflowID != *req.Msg.WorkflowId`. This is fine for existing usage, but the proposed `workflow_id` filter in the session list should handle the empty string consistently — the existing code already guards `*req.Msg.WorkflowId != ""` so the filter is only applied when non-empty.

### Session badge display performance

Adding a workflow badge to every session card means the session object now needs its `workflow_id` to be non-empty to trigger badge rendering. Since `workflow_id` is already returned in the `Session` proto (field 62 in `types.proto`), no additional RPC is needed. But if the workflow name (not just ID) needs to be shown in the badge, the frontend would need either:
- A client-side workflow-id → name lookup map (populated from `ListWorkflows` on page load), or
- A denormalized `workflow_name` field on the `Session` proto.

A map-based lookup is preferable to avoid denormalization drift.

### Review queue and workflow sessions interaction

When "suppress workflow sessions by default" is active, workflow sessions hidden from the main list can still surface in the review queue (the poller is in-memory and not filtered by UI state). Users may receive notifications about sessions they can't see. Consider whether workflow sessions should be excluded from the review queue when auto-archive is configured, or at minimum make the approval notification link navigate to the workflow page rather than the main session list.

### "Group by Workflow" display name requires workflow lookup

If the grouping groups by `workflowId` (UUID), the group header displays a UUID string, which is unusable. The display name must be the workflow name. This requires `groupSessions()` to receive a `workflowIdToName: Map<string, string>` parameter — which is not the current function signature. The function signature would need to change, which in turn would affect `strategies.test.ts`.

### Archive of one-off workflow sessions

One-off sessions (created via `OneOff: true` in `FireNow`) have `SessionType == SessionTypeOneOff` in addition to having a `WorkflowID`. The existing `maybeAutoArchive` already handles them correctly since it only checks `WorkflowID != ""`. The proposed retention sweeper must also handle them — one-off sessions are stored in `~/.stapler-squad/one-off/` directories and archiving them should either clean up or leave the directory (current behavior: directory is left in place, same as regular sessions).
