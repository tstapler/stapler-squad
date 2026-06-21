# Architecture Research: Redux Shape, Proto Fields, and Rendering Logic

## 1. Redux State Shape — Full Inventory

### Entity Adapter Fields (from `Session` proto)

`sessionsSlice.ts` uses `createEntityAdapter<Session>` which stores proto-generated `Session` objects directly in the `entities` map. Every field on the `Session` TypeScript type is in the entity. Key status-relevant fields:

| Field | Proto field | Notes |
|---|---|---|
| `status` | `session.v1.SessionStatus` enum | Lifecycle status (ACTIVE, STOPPED, PAUSED, etc.) |
| `subStatus` | `session.v1.SubStatus` enum (field 54) | Fine-grained chip display: PROCESSING, NEEDS_APPROVAL, etc. |
| `workingState` | `session.v1.WorkingState` enum (field 50) | Always UNSPECIFIED on `Session` — `InstanceToProto` never sets it |
| `rateLimitState` | `session.v1.RateLimitState` enum (field 40) | Rate limit overlay |

### Extra State (Redux-only, not in proto `Session`)

| Field | Type | Purpose |
|---|---|---|
| `detectedStatusMap` | `Record<string, { detectedStatus: string; detectedContext: string }>` | Per-session string from `StatusChangedEvent`. Drives `StatusBadge`. |
| `connectionState` | `"connected" \| "stale" \| "disconnected"` | Stream health indicator |
| `deletedIds` | `Record<string, true>` | Tombstone map — prevents stream reconnect from resurrecting deleted sessions |
| `loading` | `boolean` | Initial load state |
| `error` | `string \| null` | Last error message |

**Key finding:** `detectedStatusMap` is entirely Redux-only. It holds raw Go `DetectedStatus.String()` values (e.g., `"Active"`, `"Processing"`, `"Needs Approval"`) that come over the wire as `optional string detected_status` on `SessionStatusChangedEvent`. There is no `detectedStatus` field on the `Session` proto today.

### Actions that write to state

| Action | Writes to entity | Writes to detectedStatusMap | Notes |
|---|---|---|---|
| `setSessions` | yes (setAll) | no | Snapshot load; does not touch map |
| `upsertSession` | yes (upsertOne) | no | `sessionUpdated` stream events; map is left stale |
| `removeSession` | yes (removeOne) | no | Deletes tombstone; does not clear map entry |
| `updateSessionStatus` | yes (updateOne: status + subStatus) | yes (clear or set) | `statusChanged` stream events only |

**Critical gap:** `upsertSession` (used by `sessionUpdated` events) never touches `detectedStatusMap`. If a `sessionUpdated` event fires (e.g., session stops) while `detectedStatusMap` still holds a "Processing" entry from an earlier `statusChanged`, the badge remains stale.

---

## 2. Proto `Session` Message — Status-Relevant Fields Today

From `web-app/src/gen/session/v1/types_pb.ts` (`Session` type, lines 23–488):

**Fields on `Session` today:**
- `status: SessionStatus` (field 6) — lifecycle status
- `subStatus: SubStatus` (field 54) — fine-grained activity; set by `toProtoSubStatus()` in adapter
- `workingState: WorkingState` (field 50) — present in proto schema but **never populated by `InstanceToProto`**; always zero/UNSPECIFIED on Session entities (only `ReviewItem` uses it)

**Fields NOT on `Session` today:**
- No `detectedStatus` field (the raw Go enum value is not on the wire for sessions)
- No `detectedContext` field on Session

**`SubStatus` enum values (relevant):** UNSPECIFIED, IDLE, PROCESSING, NEEDS_APPROVAL, ERROR, TESTS_FAILING, RATE_LIMITED, INPUT_REQUIRED, READY, SUCCESS.

**`WorkingState` enum values:** UNSPECIFIED, ACTIVE, PROCESSING, IDLE, WAITING. This is a coarser grouping of `SubStatus` used only by `ReviewItem` (via `review_queue_adapter.go`), not by the session list.

---

## 3. `InstanceToProto` — Detection Serialization

Source: `/server/adapters/instance_adapter.go`

### `toProtoSubStatus(inst)` (lines 206–237)

The only detection-related serialization on `Session`:

```
if inst.Status != session.Active → SUB_STATUS_UNSPECIFIED (always clear for non-Active)
if ratelimit.StateWaiting         → SUB_STATUS_RATE_LIMITED
switch inst.GetDetectedStatus():
  StatusProcessing | StatusActive | StatusWaitingForAgent → SUB_STATUS_PROCESSING
  StatusNeedsApproval  → SUB_STATUS_NEEDS_APPROVAL
  StatusInputRequired  → SUB_STATUS_INPUT_REQUIRED
  StatusError          → SUB_STATUS_ERROR
  StatusTestsFailing   → SUB_STATUS_TESTS_FAILING
  StatusReady          → SUB_STATUS_READY
  StatusIdle           → SUB_STATUS_IDLE
  StatusSuccess        → SUB_STATUS_SUCCESS
  default              → SUB_STATUS_UNSPECIFIED
```

**Key observation:** `StatusActive` (the `.*` catch-all renamed by R2.1 to `StatusExecuting`) and `StatusProcessing` both map to `SUB_STATUS_PROCESSING`. `StatusReady` (which requirements call the `.*` fallback) maps to `SUB_STATUS_READY`. The requirements rename `StatusActive → StatusExecuting` and replace the `.*` pattern with `StatusUnknown` mapping to `SUB_STATUS_UNSPECIFIED`.

### `MapDetectedStatusToWorkingState(s)` (lines 391–406)

Only used by `review_queue_adapter.go` to populate `ReviewItem.WorkingState`. **Not called by `InstanceToProto`** — confirms the `Session.WorkingState` field is always UNSPECIFIED on wire.

---

## 4. Rendering Logic — `StatusBadge` vs `SubStatusChip`

### `SessionCard.tsx` badge rendering (lines 507–522)

```
Condition A: StatusBadge renders when
  - detectedStatus (string from detectedStatusMap) is non-empty
  - AND NOT (suppressApprovalSubStatus AND detectedStatus is "Needs Approval" or "Input Required")
  - AND session.subStatus === SubStatus.UNSPECIFIED OR session.subStatus === SubStatus.IDLE

Condition B: SubStatusChip renders when
  - session.status === SessionStatus.ACTIVE (numeric cast check)
  - AND session.subStatus !== SubStatus.UNSPECIFIED
  - AND session.subStatus !== SubStatus.IDLE
  - AND NOT suppressed approval
```

**Both can render simultaneously** if: `detectedStatusMap` has a non-"Needs Approval" entry AND `subStatus` is something other than UNSPECIFIED/IDLE. For example, if `detectedStatusMap[id] = { detectedStatus: "Active" }` (old badge-type statuses that don't overlap with SubStatus chip statuses) and `subStatus = SUB_STATUS_PROCESSING`, Condition A could be true (PROCESSING ≠ UNSPECIFIED/IDLE, so A fails). In practice the condition prevents overlap because Condition A requires `subStatus` to be UNSPECIFIED or IDLE, but this depends on both events being in sync.

**The staleness problem:** If `statusChanged` fires and sets `detectedStatusMap[id] = "Processing"`, then later `sessionUpdated` fires with the session now stopped (status=STOPPED, subStatus=UNSPECIFIED — set by adapter), `upsertSession` does NOT clear `detectedStatusMap`. The map still shows "Processing". `StatusBadge` renders because: `detectedStatus = "Processing"` is truthy, AND `subStatus = UNSPECIFIED` matches the condition. Result: a stopped session shows "Processing" badge.

### `SessionRow.tsx` badge rendering (lines 212–218)

`SessionRow` only renders `SubStatusChip` (no `StatusBadge`). It reads `detectedStatusMap` nowhere — it only uses `session.subStatus` from the entity. Row view is immune to the `detectedStatusMap` staleness bug but only shows the proto-level chip.

### `StatusBadge.tsx` — Raw string switch

`getDetectedStatusInfo(status: string)` switches on Go enum string names:
`"Ready"`, `"Processing"`, `"Needs Approval"`, `"Input Required"`, `"Error"`, `"Tests Failing"`, `"Idle"`, `"Active"`, `"Success"`. Any rename of the Go constant silently breaks the match (default case renders the raw string with unknown styling). This is exactly R6.2's concern.

---

## 5. Ideal `upsertSession` Implementation (Pseudocode for R4)

With `detectedStatus: DetectedStatus` added to the `Session` proto (R1), `upsertSession` can keep both the entity and `detectedStatusMap` in sync atomically:

```typescript
upsertSession(state, action: PayloadAction<Session>) {
  const session = action.payload;

  // Tombstone guard: don't resurrect deleted sessions
  if (state.deletedIds[session.id]) return;

  // Update the entity
  sessionsAdapter.upsertOne(state, session);

  // Sync detectedStatusMap from the proto field (R4.2, R4.3)
  if (session.status !== SessionStatus.ACTIVE) {
    // Non-active: always clear — no badge should linger (R4.2)
    delete state.detectedStatusMap[session.id];
  } else if (
    session.detectedStatus !== undefined &&
    session.detectedStatus !== DetectedStatus.UNSPECIFIED
  ) {
    // Active with detection info: update from typed field (R4.3)
    // detectedContext would need to be added to Session proto too
    state.detectedStatusMap[session.id] = {
      detectedStatus: detectedStatusEnumToString(session.detectedStatus),
      detectedContext: session.detectedContext ?? "",
    };
  }
  // Active but UNSPECIFIED detectedStatus: leave map unchanged
  // (partial update — don't clobber an earlier detection from statusChanged)
}
```

**Alternative (post-R6):** After `StatusBadge` is migrated to use the typed `DetectedStatus` enum directly (R6.2), `detectedStatusMap` can be removed entirely. `StatusBadge` would read `session.detectedStatus` from the entity directly. `upsertSession` would just be `sessionsAdapter.upsertOne` with the tombstone guard — no map management needed.

---

## Key Findings Summary

1. **`detectedStatusMap` is the only disjoint state piece**: The entity holds `subStatus` (proto enum, type-safe) while `detectedStatusMap` holds raw Go string names. `upsertSession` never touches `detectedStatusMap`, so any `sessionUpdated` event that changes session status without a paired `statusChanged` leaves the badge stale. Adding `detectedStatus: DetectedStatus` to the `Session` proto and syncing it inside `upsertSession` eliminates this split.

2. **`WorkingState` on `Session` is a phantom field**: The `Session` proto has `working_state` (field 50) but `InstanceToProto` never sets it — it's always UNSPECIFIED on session entities. `WorkingState` is computed and set only on `ReviewItem` via `review_queue_adapter.go`. R5 (move derivation to frontend) needs to target `ReviewItem.workingState`, not `Session.workingState`, since the latter is already effectively frontend-computed (it's zero).

3. **`StatusBadge` has a dual-render risk and a raw-string fragility**: In `SessionCard`, `StatusBadge` (driven by `detectedStatusMap` string) and `SubStatusChip` (driven by proto `subStatus` enum) can theoretically co-render if the two state tracks diverge. The guard condition `subStatus === UNSPECIFIED || subStatus === IDLE` prevents most cases but relies on both tracks being current. `StatusBadge.getDetectedStatusInfo()` switches on Go enum string values (`"Active"`, `"Processing"`, etc.) — renaming any Go constant silently degrades to the unknown/grey variant with zero compile-time protection.
