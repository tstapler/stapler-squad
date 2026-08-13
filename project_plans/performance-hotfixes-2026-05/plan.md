# Performance Hotfix Plan — 2026-05-02

Source: live pprof session at `http://localhost:6060` (server running with `--profile`).

---

## Summary

Five concrete hotspots identified from mutex, block, and allocs profiles. All are in
application code (not library or OS). Combined mutex cycle count across the top three
logging-related issues: ~6.5B cycles. Block count on the WebSocket streaming goroutine:
26,437 events.

---

## PerfFix-1: Remove `log.Printf` from `GetStatus` hot path

**Profile**: mutex — `session/instance_status.go:78`
**Signal**: 2.2B cycles, 5094 events
**Root cause**: `GetStatus` is called by `ReviewQueuePoller.checkSession` on every poll
cycle for every session. It calls `log.DebugLog.Printf` unconditionally. stdlib `log.Printf`
acquires a mutex on `(*Logger).output` for every call, even when no one reads the output.
**Fix**: Remove the debug printf entirely from `GetStatus`. It was left in to diagnose a
controller detection issue (see the comment on line 78); the issue is long resolved.
```go
// DELETE these lines from session/instance_status.go:78-80:
log.DebugLog.Printf("[GetStatus] Session '%s': exists=%v, controller!=nil=%v, IsStarted=%v",
    instance.Title, exists, controller != nil, exists && controller != nil && controller.IsStarted())
```
**Enforcement**: golangci-lint rule `no-debug-log-in-hot-poll` — flags `log.DebugLog.Printf`
calls in functions named `GetStatus`, `checkSession`, `checkSessions`, `getContent`,
`pollLoop` without a `if log.DebugLog != nil` guard. Test: fires on pre-fix code.
**Estimated impact**: HIGH — 5094 mutex acquisitions per poll cycle eliminated.

---

## PerfFix-2: Gate `log.DebugLog.Printf` in `ReviewQueuePoller.getContent`

**Profile**: mutex — `session/review_queue_poller.go:557,574,581`
**Signal**: 1.4B cycles, 2607 events
**Root cause**: `getContent` logs on every cache hit and miss without checking `DebugLog != nil`
first (line 618 in `checkSession` also has an unconditional call).
**Fix**: Wrap each `log.DebugLog.Printf` with `if log.DebugLog != nil`:
```go
// review_queue_poller.go:618 — wrap this:
if log.DebugLog != nil {
    log.DebugLog.Printf("[ReviewQueue] Session '%s': LastMeaningfulOutput is zero...", inst.Title)
}
// Same pattern for lines 557, 574, 581
```
**Enforcement**: Same lint rule as PerfFix-1.
**Estimated impact**: MEDIUM — 2607 mutex acquisitions eliminated from content cache path.

---

## PerfFix-3: Remove `log.Printf` from control mode `%output` event handler

**Profile**: mutex — `session/tmux/control_mode.go:331`
**Signal**: 2.7B cycles, 94 events (94 separate contention windows, each holding the lock
a long time — the control mode reader is otherwise very fast, so this dominates its latency)
**Root cause**: Line 331 logs every `%output` event with `DebugLog.Printf`. The guard
`if log.DebugLog != nil` IS present, but when debug logging is enabled the lock is held
during format + write on the critical terminal-output path.
**Fix**: This log is genuinely useful for debugging but is too noisy for production. Replace
with a counter that can be read by the debug menu:
```go
// Replace the log.Printf with an atomic counter increment
t.outputEventCount.Add(1)
// Expose via debug endpoint or log only every Nth event
```
Alternatively: demote to trace-level only (a separate `TraceLog` that can be disabled
independently of DebugLog).
**Enforcement**: Benchmark `BenchmarkControlModeOutput` that asserts `ns/op < 1000` with
logging disabled. Must regress (>5000 ns/op) when the Printf is re-introduced without the guard.
**Estimated impact**: MEDIUM — eliminates log mutex from terminal I/O hot path.

---

## PerfFix-4: Remove per-frame `log.Printf` from WebSocket streaming goroutine

**Profile**: block — `server/services/connectrpc_websocket.go:629`
**Signal**: 23T cycles, 26,437 events (extremely high block count for a per-connection goroutine)
**Root cause**: Line 629 in `streamViaControlMode` calls `log.DebugLog.Printf` inside the
`select case data := <-updateChan` arm — every time a terminal update arrives. At high tmux
output rates this fires hundreds of times per second. Each call: (a) acquires the log mutex,
(b) formats the string, (c) writes to the log file. The goroutine blocks in `selectgo` while
waiting for the next update, and the mutex contention during the log write explains the
abnormally high block count.
**Fix**: Remove the line entirely. The byte count is observable from the envelope send anyway.
```go
// DELETE line 629-630 from connectrpc_websocket.go:
log.DebugLog.Printf("[streamViaControlMode] Sent update (%d bytes) for session '%s'",
    len(data), sessionID)
```
**Enforcement**: Benchmark `BenchmarkStreamViaControlMode_HighFrequency` using a mock
`updateChan` that delivers 10,000 updates and measures total time + allocs. Must show
allocs/op == 0 for the log-free path.
**Estimated impact**: HIGH — 26,437 unnecessary goroutine wake-ups + mutex acquisitions per
connection removed from the critical streaming path.

---

## PerfFix-5: Replace read-modify-write with direct UPDATE in `updateFieldInRepo`

**Profile**: allocs — `session/ent_repository.go:622` via `session/storage.go:285`
**Signal**: Full row read (`ent.(*SessionQuery).sqlAll`) before every single-field update.
Called on every `UpdateInstanceLastUserResponse` event.
**Root cause**: `updateFieldInRepo` calls `EntRepository.Get` to fetch the full session entity,
then calls `Save()`. This issues a SELECT + UPDATE when only an UPDATE is needed.
**Fix**: Use ent's `Update().Where(session.ID(id)).SetField(value).Exec(ctx)` directly:
```go
// Instead of Get → mutate → Save:
func (r *EntRepository) UpdateLastUserResponse(ctx context.Context, id int, t time.Time) error {
    return r.client.Session.UpdateOneID(id).SetLastUserResponse(t).Exec(ctx)
}
```
Introduce typed update methods for frequently-updated fields rather than the generic
`updateFieldInRepo` helper.
**Enforcement**: Integration test `TestEntRepository_UpdateField_NoSelectIssued` that
intercepts SQL statements via an ent Hook and asserts that no SELECT is issued for a
single-field update.
**Estimated impact**: MEDIUM — eliminates one SELECT per user-response event from the
write path; reduces ent ORM allocation pressure.

---

## Enforcement Summary

| Fix | Enforcement type | Implementation |
|-----|-----------------|----------------|
| PerfFix-1 | Lint rule | `no-debug-log-in-hot-poll` in golangci-lint custom rules |
| PerfFix-2 | Lint rule | Same rule as PerfFix-1 |
| PerfFix-3 | Benchmark | `BenchmarkControlModeOutput` with ns/op gate |
| PerfFix-4 | Benchmark | `BenchmarkStreamViaControlMode_HighFrequency` allocs gate |
| PerfFix-5 | Integration test | `TestEntRepository_UpdateField_NoSelectIssued` |

---

## Reflect & Fix Classification

| Fix | Category | Why it slipped through | Earliest detection |
|-----|----------|----------------------|-------------------|
| PerfFix-1,2,3,4 | **Semantic/Intent** | `log.Printf` is syntactically valid in a loop; no rule prevented it | Lint rule (level 2) |
| PerfFix-5 | **API Contract Gap** | `updateFieldInRepo` generic helper hides the SELECT; callers can't see it | Integration test (level 4) |

---

## Implementation Status

| Fix | Status | Notes |
|-----|--------|-------|
| PerfFix-1 | ✅ DONE | `instance_status.go:78` — removed diagnostic Printf left from resolved issue |
| PerfFix-2 | ✅ DONE | `review_queue_poller.go` — removed 4 hot-path debug Printf calls |
| PerfFix-3 | ✅ already guarded | `control_mode.go:330` had `if log.DebugLog != nil` guard; only fires when debug enabled |
| PerfFix-4 | ✅ DONE | `connectrpc_websocket.go:629` — removed per-frame debug Printf from streaming goroutine |
| PerfFix-5 | ⏳ DEFERRED | `LastUserResponse` is NOT in the ent schema — field is never persisted across restarts; the `updateFieldInRepo` round-trip is a no-op. Full fix requires adding `last_user_response` to ent schema and running `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`. Spawn a dedicated agent with schema context. |

## Suggested Next Steps

1. **Write the lint rule** — custom golangci-lint rule `no-debug-log-in-hot-poll` that
   flags `log.DebugLog.Printf` in polling/streaming functions. One agent, ~2 hours.
2. **PerfFix-5 proper fix** — add `last_user_response`, `processing_grace_until`,
   `last_prompt_detected` to ent schema; add `UpdateLastUserResponse(ctx, title, t)` method
   to `EntRepository` like `UpdateTimestamps`. Requires ent schema migration.
3. **PerfFix-3 enhancement** — consider demoting %output debug log to a per-Nth-event
   counter to avoid log mutex on high-frequency output streams.
