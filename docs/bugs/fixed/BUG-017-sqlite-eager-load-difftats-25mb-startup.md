# BUG-017: SQLite Eager-Loads All DiffStats at Startup, Pinning 25.5 MB [SEVERITY: Low]

**Status**: ✅ FIXED (2026-04-23)
**Discovered**: 2026-04-23 — via pprof heap profile
**Fixed**: 2026-04-23 — `session/ent_repository.go`
**Impact**: Every startup materializes **25.5 MB** of Go string data from SQLite by eagerly loading all session rows including full `DiffStats` associations. This memory stays pinned for the process lifetime. Worsens with session count.

## Problem Description

Pprof heap profile shows the single largest in-use allocation:

```
25.5 MB in-use
session.(*EntRepository).List
  → ent.(*DiffStatsQuery).sqlAll
    → go-sqlite3._Cfunc_GoStringN  (C → Go string copy)
```

On startup, `EntRepository.List` loads all sessions with an eager join to `DiffStats`. The `go-sqlite3` driver copies every string column from SQLite's C memory into the Go heap via `C.GoStringN`. Because the loaded `ent` objects are retained in the session registry, the 25.5 MB stays resident.

This is the dominant heap consumer at idle (40.7 MB total Go heap; 25.5 MB is 63% of it).

## Reproduction Steps

1. Run stapler-squad with `--profile` (with a reasonable number of sessions)
2. `curl -s 'localhost:6060/debug/pprof/heap?debug=1' > heap.txt`
3. Look for `ent.(*DiffStatsQuery).sqlAll` and `go-sqlite3._Cfunc_GoStringN`
4. Expected: only session metadata loaded at startup; DiffStats fetched on demand
5. Actual: full DiffStats join materialized for every session at startup

## Root Cause

The startup call to `EntRepository.List` uses an Ent eager-load edge (`.WithDiffStats()` or equivalent) that issues a JOIN or secondary query to fetch `DiffStats` for all sessions in one pass. This is efficient for SQL round-trips but wasteful for memory when DiffStats is only needed for individual session detail views.

## Files Likely Affected

- `session/storage.go` or `session/ent_repository.go` — `List()` call site, check for `.WithDiffStats()` eager load
- `session/ent/` — generated Ent query builders

## Fix Approach

**Option A (Preferred)**: Remove the eager `DiffStats` load from the startup `List()` call. Load DiffStats lazily when a specific session's detail is requested (e.g., on `GetSession` RPC or when the web UI opens a session card).

**Option B**: Load only DiffStats summary fields (added/removed counts) at startup; defer loading the full diff content until needed.

**Option C**: Accept as-is if 25.5 MB is acceptable relative to the system's available RAM and the session count is not expected to grow significantly.

Option A is lowest risk: Ent makes it straightforward to remove an eager-load edge. The DiffStats data is still available on demand via the session's Ent client.

## Verification

After fix, heap profile should show `ent.(*DiffStatsQuery).sqlAll` absent from the top allocators at startup. Total Go heap at idle should drop from ~40 MB toward ~15 MB.

## Related Tasks

- Discovered alongside BUG-016 (WebSocket compression) during pprof analysis session 2026-04-23
- Related to historical BUG-003 (large state file from diff content) — both stem from over-eager diff data loading
