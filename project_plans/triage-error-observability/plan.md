# Plan: triage-error-observability

## Approach

Three independent tracks, each shippable alone:

1. **Track A – Durable error detail (AC-1, AC-2, AC-3)**: Add an `error_detail` field to the
   `item_session` ent schema, populate it for `other`-bucket failures in `TriggerTriage` and
   `TriggerReReview`, expose it in the proto and UI session history panel.

2. **Track B – Uniform log coverage (AC-5)**: Audit every `classifyHeadlessCallError` call site to
   ensure the underlying error is always logged on the same structured log call as `errType`.

3. **Track C – Batch parking escalation (AC-4, AC-6)**: Add a rate-counted notification when
   ≥ N items park within a window, plus a runbook entry.

---

## Track A: Durable error detail

### A1. Add `error_detail` field to ent schema

**File:** `session/ent/schema/item_session.go`

Add to `Fields()`:
```go
field.String("error_detail").
    Optional().
    Default("").
    Comment("For end_reason=\"other\": the truncated (≤500 chars) Go error text from classifyHeadlessCallError's catch-all, persisted so a post-incident DB query can recover it after log rotation. Empty for all other end_reason buckets and for successful sessions."),
```

Run `make ent-gen` to regenerate. Commit only the schema file (generated files are `.gitignore`d).

### A2. Extend storage method

**File:** `session/storage_backlog.go`

Add a new method (do NOT change `UpdateItemSessionEndedWithReason`'s signature — it has callers
outside the `other` path that don't need a detail string):

```go
// UpdateItemSessionEndedWithDetail is like UpdateItemSessionEndedWithReason but also
// sets error_detail — called only when classifyHeadlessCallError returns "other" so
// the underlying error text survives log rotation.
func (r *EntRepository) UpdateItemSessionEndedWithDetail(ctx context.Context, id string,
    endedAt time.Time, reason, detail string) error
```

Also add corresponding method to the `Storage` facade and the storage interface
(`session/storage_interface.go` or equivalent).

### A3. Wire in TriggerTriage and TriggerReReview

**File:** `server/services/backlog_service_trigger_triage.go:552`

Change:
```go
_ = s.storage.UpdateItemSessionEndedWithReason(cleanupCtx, isID, time.Now(), errType)
```
to:
```go
if errType == "other" {
    detail := truncateErrorDetail(callErr, 500)
    _ = s.storage.UpdateItemSessionEndedWithDetail(cleanupCtx, isID, time.Now(), errType, detail)
} else {
    _ = s.storage.UpdateItemSessionEndedWithReason(cleanupCtx, isID, time.Now(), errType)
}
```

Same change in `TriggerReReview` (`backlog_service_triage.go:3129`).

Add a small helper:
```go
// truncateErrorDetail returns err.Error() truncated to maxLen runes, or "" if err is nil.
func truncateErrorDetail(err error, maxLen int) string {
    if err == nil { return "" }
    s := err.Error()
    if len(s) <= maxLen { return s }
    return s[:maxLen]
}
```

### A4. Add proto field

**File:** `proto/session/v1/backlog.proto`

In the `ItemSession` message, after field 18 (`end_reason`):
```proto
// error_detail carries the truncated underlying Go error text when end_reason is "other" —
// the catch-all bucket that previously gave no further diagnostic signal after log rotation.
// Empty for all other end_reason values and for successful sessions.
string error_detail = 19;
```

Run `make proto-gen`.

### A5. Populate proto mapper

**File:** `server/services/backlog_service_triage.go` (or wherever `itemSessionToProto` lives)

Add `ErrorDetail: is.ErrorDetail` (or equivalent ent-generated accessor) to the mapper.

### A6. Update stuck-context generation

**File:** `session/backlog_lifecycle_triage.go`

In `reconcileOrphanedTriageItems` where the context string is built, incorporate `EndReason` AND
`ErrorDetail` (when non-empty):
```go
context := fmt.Sprintf("triage session %s ended (%s) without moving the item out of idea", is.SessionUUID, is.EndReason)
if is.ErrorDetail != "" {
    context += ": " + is.ErrorDetail
}
```

### A7. Unit test for `other`-bucket persistence

**File:** `server/services/backlog_service_triage_test.go` or new `_test.go`

Add a test:
- Inject a fake headless runner that returns an unclassified error (e.g. `errors.New("dial tcp: connection refused")`)
- Trigger triage
- Assert the stored ItemSession has `end_reason="other"` AND `error_detail` contains `"dial tcp: connection refused"`

---

## Track B: Uniform log coverage

### B1. Audit all classifyHeadlessCallError call sites

There are currently 2 call sites:
1. `TriggerTriage` (line 550–551): already logs `"error", callErr` ✓
2. `TriggerReReview` (line 3119): already logs `"error", callErr` ✓

Both are correct. No changes needed for log coverage. AC-5 is satisfied without code changes.

However: document in the function's doc comment that callers MUST log `"error", callErr` alongside
`errType`, not just the bucket name, to prevent regressions at future call sites.

---

## Track C: Batch parking escalation

### C1. Add batch-park counter to BacklogLifecycleListener

**File:** `session/backlog_lifecycle.go` or `session/backlog_lifecycle_triage.go`

Add to `BacklogLifecycleListener`:
```go
recentParkMu  sync.Mutex
recentParks   []time.Time  // timestamps of recent justParked=true events
```

In `retryOrphanedTriageWithBackoffGate`, after the per-item notify:
```go
l.recentParkMu.Lock()
now := time.Now()
cutoff := now.Add(-batchParkWindow) // default 10m
l.recentParks = append(l.recentParks, now)
// prune old entries
n := 0
for _, t := range l.recentParks {
    if t.After(cutoff) {
        l.recentParks[n] = t
        n++
    }
}
l.recentParks = l.recentParks[:n]
count := len(l.recentParks)
l.recentParkMu.Unlock()

if count >= batchParkThreshold {
    l.notify("", // no specific item
        "Multiple auto-triage retries exhausted",
        fmt.Sprintf("%d items have hit the retry cap within the last %v — they may share a root cause (check logs for errType=other around %v). Use Reset on each to restart auto-triage.", count, batchParkWindow, now.Format(time.RFC3339)),
        8, true, true,
    )
}
```

Constants:
```go
const batchParkThreshold = 3
const batchParkWindow    = 10 * time.Minute
```

### C2. Runbook entry

**File:** `docs/how-to/debug-with-logs.md`

Add section: **Diagnosing parked triage items (`errType=other`)**:
- Query pattern: `SELECT id, error_detail FROM item_sessions WHERE end_reason='other' AND ended_at > '...'`
- What `error_detail` contains (truncated Go error text)
- How to reset a batch: Use the Reset action on each item in the UI, or call the Reset RPC
- How to correlate batch failures (look for common `error_detail` prefix)

---

## Implementation Order

1. Track A (AC-1 through AC-3): ent schema change → storage method → wire call sites → proto →
   mapper → stuck-context → unit test
2. Track B (AC-5): doc comment only, no code change needed
3. Track C (AC-4, AC-6): in-memory counter + runbook

Tracks A and C are independent and can be implemented in parallel by two engineers.

## Estimated Effort

| Task | Estimate |
|---|---|
| A1–A3: schema + storage + wire | 3h |
| A4–A5: proto + mapper | 1h |
| A6: stuck-context update | 30m |
| A7: unit test | 2h |
| B1: doc comment | 30m |
| C1: batch counter | 2h |
| C2: runbook | 1h |
| **Total** | **~10h** |
