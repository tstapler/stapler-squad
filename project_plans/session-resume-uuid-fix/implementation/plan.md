# Implementation Plan: Session Resume UUID Fix

## Root Cause

`HistoryLinker.correlateSession()` in `session/history_linker.go` overwrites a
paused/hibernated session's stored conversation UUID via the path-based `DetectByPath`
fallback. When other sessions run in the same directory after the pause, their newer
JSONL files become the "most recently modified" and get returned instead of the
paused session's correct file.

## Change

**File:** `session/history_linker.go`

Gate the path-based fallback with:
```go
pathFallbackAllowed := !alreadyLinked ||
    (inst.Status != Paused && inst.Status != Hibernated)
if info == nil && pathFallbackAllowed {
    // DetectByPath...
}
```

**Why this condition:**
- `!alreadyLinked` → unlinked sessions always use path fallback (best-effort linking)
- `inst.Status != Paused && inst.Status != Hibernated` → active/running sessions use
  path fallback (needed so `/clear` UUID changes are detected via the path heuristic
  in tests; in production, PID detection catches these first anyway)
- Paused + already linked → skip path fallback → stored UUID is preserved

## Tests

**New tests in `session/history_linker_test.go`:**
- `TestHistoryLinker_CorrelateSession_PausedSession_PreservesUUID` — core bug regression
- `TestHistoryLinker_CorrelateSession_HibernatedSession_PreservesUUID` — same for Hibernated

**Existing tests that must still pass:**
- `TestHistoryLinker_CorrelateSession_Force_UpdatesUUIDAfterClear` — running session
  still gets UUID updated when a newer file appears (inst.Status = Running)
- All other HistoryLinker tests
