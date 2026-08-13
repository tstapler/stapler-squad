# Adversarial Review: memory-pressure-ux

**Date**: 2026-05-21
**Verdict**: CONCERNS
**Plan revision**: 2026-05-21 — all CONCERNS addressed in plan.md v2; verdict stands at CONCERNS (no blockers found)

## Blockers

*(None)*

## Concerns

- [ ] **`MemoryCacheReader` interface / cache sharing is underspecified** — Task 1.3.2a describes injecting a `MemoryCacheReader` into `SessionService` but doesn't specify where the shared cache instance lives or how it's constructed. The sweeper owns the cache (it fills it), but `SessionService` needs read access. The plan says "wired in server/server.go" but gives no concrete wiring pattern. During implementation this could cause either a circular dependency (`session` importing `server`) or an awkward package split. **Recommendation**: explicitly state that `HibernationSweeper` exposes a `MemoryCacheReader()` method returning an interface, and that `server.go` passes `sweeper.MemoryCacheReader()` to `SessionService` — all in the same package chain with no circular deps.

- [ ] **WatchSessions / event stream does not populate memory fields** — The plan only wires `system_memory_pct` into `ListSessions`. But the architecture research notes that the session event stream (WatchSessions) drives live UI updates. If `WatchSessions` pushes `SessionUpdatedEvent` without the new memory fields, the UI will show 0 after the first watch event overwrites the list result. **Recommendation**: add an explicit task to populate `memory_rss_mb` and `estimated_savings_mb` when constructing `SessionEvent` payloads in `WatchSessions` (or in the event publisher used by the sweeper on hibernation).

- [ ] **`useSessionService` extraction of `systemMemoryPct` is underspecified** — Task 2.1.1b says "extract `systemMemoryPct` from `ListSessionsResponse` when `listSessions` resolves" but the service currently uses a WebSocket/SSE watch transport (`createWatchTransport`) for live updates, not polling. The `ListSessionsResponse` is only fetched on initial load and explicit `listSessions()` calls. After the first load, updates come via `SessionEvent`s which carry individual `Session` objects, not the list-level `systemMemoryPct`. This means `systemMemoryPct` goes stale after the initial load. **Recommendation**: either (a) also extract `systemMemoryPct` from watch events if the server encodes it there, or (b) explicitly state that `listSessions()` is called on a timer (e.g., every 30s) to refresh system memory, or (c) move `systemMemoryPct` to individual `Session` events.

- [ ] **`NewHibernationSweeper` signature change breaks existing callers** — Task 1.2.1a updates `NewHibernationSweeper` to add a `memory.Reader` parameter. `server/server.go` line ~495 is the only caller mentioned. If there are test files that construct a `HibernationSweeper` directly (e.g., integration tests), they will fail to compile. **Recommendation**: identify all call sites of `NewHibernationSweeper` before starting; the task should explicitly list all files that need updating, not just `server/server.go`.

- [ ] **RSS values never populate on first ListSessions** — The memory cache is filled by the sweeper, which runs every 5 minutes. On a fresh server start, the cache is empty. `ListSessions` will return 0 for all `memory_rss_mb` fields for up to 5 minutes. The plan doesn't address this cold-start gap. **Recommendation**: add a `fetchInitialMemory()` call in `HibernationSweeper.Start()` (or a separate `ReadOnce()` method) that populates the cache immediately on startup, before the first 5-minute tick.

## Minors

- The plan references `vars.color.warningText` in the CSS, but only lists `vars.color.warning` and `vars.color.warningBg` in the theme contract table in architecture.md. Verify `warningText` exists in `theme-contract.css.ts` before using it.
- Task 1.1.1b says "cap at 50 PIDs per session" but no test covers the cap behavior — add a test case for the depth-3 + 50-PID cap to ensure it doesn't silently miss large process trees.
- `MemoryPressureCallout` is placed inline in `SessionList` but `SessionList.tsx` is already 38KB. Consider extracting to a sub-component file to keep the file manageable.
- The plan doesn't specify which `SessionStatus` import to use when checking `session.status === SessionStatus.HIBERNATED` in the frontend (the generated proto enum vs. a string literal). Clarify that the generated `SessionStatus` enum from `@/gen/session/v1/types_pb` should be used.
- FR-5.2 in requirements says badge uses `statusWarning` color token. Plan correctly notes this token doesn't exist and uses `vars.color.warning` instead. This is a requirements artifact — good catch, but worth noting the discrepancy exists.
- Task 3.1.1a mentions "fix any compilation errors" as a task step — these should be resolved during individual tasks, not deferred to a cleanup pass. Treat this as a reminder, not a task.
