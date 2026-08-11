# Implementation Plan: memory-pressure-ux

**Feature**: Surface per-session memory usage, fix resource-pressure hibernation, and add pressure UX indicators
**Date**: 2026-05-21
**Status**: Ready for implementation
**ADRs**: None (all decisions use existing patterns; no non-standard technology choices)

---

## Dependency Visualization

```
Phase 1: Backend Foundation
  Task 1.1.1a (session/memory package + Reader interface)
      └── Task 1.1.1b (GopsutilReader implementation + PIDResolver)
              └── Task 1.2.1a (sweeper cache struct)
                      └── Task 1.2.1b (sweeper resource-pressure logic)
                              └── Task 1.3.1a (proto field additions)
                                      └── Task 1.3.1b (make generate-proto) ← GATE
                                              └── Task 1.3.2a (InstanceToProto adapter)
                                                      └── Task 1.3.2b (ListSessions system_memory_pct)

Phase 2: Frontend (starts after Task 1.3.1b)
  Task 2.1.1a (SystemMemoryContext)
      └── Task 2.1.2a (MemoryNavBadge component + Header wiring)
  Task 2.2.1a (SessionCard/SessionRow memory display)
      └── Task 2.2.1b (SessionActionsOverflow tooltip update)
  Task 2.3.1a (MemoryPressureCallout component)
      └── Task 2.3.1b (SessionCard pressure-highlight CSS variant)

All Phase 2 tasks are independent of each other (only depend on proto gate).
```

---

## Phase 1: Backend

### Epic 1.1: Memory Measurement Package

**Goal**: Create a testable `session/memory` package that reads system and per-process RSS using gopsutil, with a mockable interface for unit tests.

#### Story 1.1.1: Memory Reader Interface and Implementation

**As a** backend developer, **I want** a `memory.Reader` interface with a gopsutil implementation, **so that** the sweeper and tests can inject real or fake readers without spawning `/proc` or tmux.

**Acceptance Criteria**:
- `session/memory` package compiles on Linux and macOS
- `GopsutilReader.SystemMemory()` returns `UsedPct` via `mem.VirtualMemory()`; returns 0.0 on error
- `GopsutilReader.ProcessMemory()` recursively walks `proc.Children()` to depth 3 and sums RSS in MB
- `PIDResolver` function type is injectable; default implementation runs `tmux list-panes -t <name> -F '#{pane_pid}'`
- All logic is unit-testable with `FakeReader` (no real tmux or `/proc` required)
- macOS returns 0 from `SystemMemory` if `mem.VirtualMemory()` errors (NFR-2)

**Files**:
- `session/memory/reader.go` (new)
- `session/memory/reader_test.go` (new)

##### Task 1.1.1a: Define package types and interfaces (~3 min)

- Create `session/memory/reader.go`
- Define `SystemStats{UsedPct float64}`, `ProcessStats{RSSMB int64}`
- Define `Reader` interface:
  ```go
  type Reader interface {
      SystemMemory(ctx context.Context) (SystemStats, error)
      ProcessMemory(ctx context.Context, pids []int32) (ProcessStats, error)
  }
  ```
- Define `PIDResolver` function type: `type PIDResolver func(ctx context.Context, tmuxSessionName string) ([]int32, error)`
- Define `FakeReader` struct for tests (exported, in same file or test file):
  ```go
  type FakeReader struct {
      SystemPct float64
      ProcRSSMB int64
      SystemErr error
      ProcErr   error
  }
  ```
- Files: `session/memory/reader.go`

##### Task 1.1.1b: Implement GopsutilReader (~5 min)

- Add `GopsutilReader` struct with `pidResolver PIDResolver` field
- Implement `SystemMemory`: call `mem.VirtualMemory()`, return `SystemStats{UsedPct: stat.UsedPercent}`, return `SystemStats{}` (zero) on any error — sentinel 0 disables pressure hibernation (see Pitfall 6)
- Implement `ProcessMemory(ctx, pids []int32)`:
  - For each shell PID: create `process.NewProcess(pid)`, call `proc.Children()` recursively to depth 3 (cap at 50 total PIDs to prevent runaway), collect all PIDs
  - For each collected PID: call `proc.MemoryInfo()`, accumulate `RSS` bytes
  - Convert total bytes to MB: `totalRSSMB = totalBytes / (1024 * 1024)`
  - Treat individual PID errors as benign (PID may have exited); continue loop
  - Return `ProcessStats{RSSMB: totalRSSMB}`
- Implement default `TmuxPIDResolver`:
  - Runs `tmux list-panes -t <tmuxSessionName> -F '#{pane_pid}'` via `exec.CommandContext` with 2s timeout
  - Parses output lines as `int32` PIDs
  - Returns empty slice (not error) if tmux session not found
- Add `NewGopsutilReader(resolver PIDResolver) *GopsutilReader`; pass `nil` → uses `TmuxPIDResolver`
- Files: `session/memory/reader.go`

##### Task 1.1.1c: Unit tests for reader (~4 min)

- Create `session/memory/reader_test.go`
- Test `FakeReader.SystemMemory` returns configured `SystemPct`
- Test `FakeReader.ProcessMemory` returns configured `ProcRSSMB`
- Test `GopsutilReader.SystemMemory` with an injected function that returns a controlled `VirtualMemoryStat` (use a mock, or test the zero-on-error path)
- Test recursive child walk: verify depth-3 cap prevents infinite loops (use a fake process tree with a `FakeChildResolver`)
- Test `TmuxPIDResolver` returns empty slice when tmux exits non-zero (no real tmux needed — mock the exec)
- Files: `session/memory/reader_test.go`

---

### Epic 1.2: Resource-Pressure Hibernation in Sweeper

**Goal**: Wire the `memory.Reader` into `HibernationSweeper`, add a 30-second per-session RSS cache, and implement the one-session-per-tick pressure hibernation logic from FR-2.

#### Story 1.2.1: Sweeper Memory Cache and Pressure Loop

**As a** system operator, **I want** the hibernation sweeper to auto-hibernate the longest-idle active session when memory pressure exceeds the threshold, **so that** the application frees RAM automatically without user intervention.

**Acceptance Criteria**:
- Sweeper reads `system_memory_pct` at each tick via injected `memory.Reader`
- When `usedPct >= ResourcePressureThreshold` and `ResourcePressureThreshold > 0`, sweeper hibernates the single longest-idle active session that had no meaningful output in the last 5 minutes
- Hibernated session has checkpoint reason set to `"resource_pressure"`
- Log line includes `reason=resource_pressure` (FR-2.3 / AC-1)
- If `usedPct == 0` (macOS sentinel), pressure hibernation is skipped entirely
- Cache entries expire after 30 seconds; immediately invalidated after a successful hibernate
- Sweeper unit tests pass with `FakeReader`; no real tmux or `/proc` needed
- All existing sweeper tests continue to pass (`make test`)

**Files**:
- `session/hibernation_sweeper.go` (modified)
- `session/hibernation_sweeper_test.go` (new or modified)

##### Task 1.2.1a: Add memory cache struct to sweeper (~3 min)

- Add `sessionMemoryCache` struct with `sync.Mutex`, `entries map[string]memoryCacheEntry` (keyed by session UUID string)
- Add `memoryCacheEntry{rssMB int64; fetchedAt time.Time}`
- Add `const cacheTTL = 30 * time.Second`
- Add method `(c *sessionMemoryCache) GetOrFetch(ctx, uuid, fetchFn) int64` — returns cached value if within TTL, else calls `fetchFn`, stores result
- Add method `(c *sessionMemoryCache) Invalidate(uuid string)`
- Add `memCache *sessionMemoryCache` field to `HibernationSweeper`
- Add `memReader memory.Reader` field to `HibernationSweeper`
- Update `NewHibernationSweeper` signature:
  ```go
  func NewHibernationSweeper(storage *Storage, cfg *appconfig.Config, memReader memory.Reader) *HibernationSweeper
  ```
- Update constructor call in `server/server.go` line ~495 to pass `memory.NewGopsutilReader(nil)` as the third argument
- **Cold-start fix**: in `HibernationSweeper.Start()`, call `s.sweep(ctx)` once immediately before starting the ticker, so the RSS cache is populated at server startup rather than waiting 5 minutes for the first tick. This ensures `ListSessions` returns real values from the first request.
- Files: `session/hibernation_sweeper.go`, `server/server.go`

##### Task 1.2.1b: Implement resource-pressure sweep logic (~5 min)

- In `sweep()`, after the existing idle-timeout loop, add a new block:
  ```go
  // Resource-pressure hibernation (FR-2)
  if s.cfg.Hibernation.ResourcePressureThreshold > 0 {
      s.sweepResourcePressure(ctx, instances)
  }
  ```
- Implement `sweepResourcePressure(ctx, instances)`:
  1. Call `s.memReader.SystemMemory(ctx)` → get `usedPct`
  2. If `usedPct == 0` return (macOS / unavailable — Pitfall 6)
  3. If `usedPct < float64(s.cfg.Hibernation.ResourcePressureThreshold)` return (below threshold)
  4. Find all Active instances where `TimeSinceLastMeaningfulOutput(inst.CreatedAt) >= 5*time.Minute`
  5. Sort candidates by `TimeSinceLastMeaningfulOutput` descending (longest idle first)
  6. Hibernate the first candidate:
     - `inst.SetHibernateReason("resource_pressure")`
     - Call `inst.Hibernate(ctx)`; on error log and return
     - `s.memCache.Invalidate(inst.UUID.String())`
     - Call `s.storage.SaveInstances(instances)`
     - Log: `"auto-hibernating idle session"` with `session=inst.Title`, `reason=resource_pressure`, `memory_pct=usedPct`
     - Return after one hibernation (one-at-a-time per tick — FR-2.2)
- Files: `session/hibernation_sweeper.go`

##### Task 1.2.1c: Sweeper unit tests for pressure hibernation (~5 min)

- Create or extend `session/hibernation_sweeper_test.go`
- Test cases (all use `FakeReader`, fake instances with controlled `LastMeaningfulOutput`):
  - **Below threshold**: `SystemPct=80`, threshold=85 → no pressure hibernate called
  - **Zero pct sentinel**: `SystemPct=0`, threshold=85 → no pressure hibernate called
  - **Above threshold, one eligible idle session**: `SystemPct=90`, one session idle 10min → that session hibernated, log has `reason=resource_pressure`
  - **Above threshold, no eligible session** (all had output < 5min ago): → none hibernated
  - **Two idle sessions**: `SystemPct=90`, sessions A (idle 20min), B (idle 10min) → only A hibernated per tick
  - **Cache invalidated on hibernate**: after hibernate, `memCache.GetOrFetch` for that UUID calls fetchFn again
  - **Threshold=0 disables pressure hibernation**: sweeper skips the pressure block
- Files: `session/hibernation_sweeper_test.go`

---

### Epic 1.3: Proto Fields and API Exposure

**Goal**: Extend the proto schema with memory fields, regenerate bindings, and populate them in `ListSessions` and `InstanceToProto`.

#### Story 1.3.1: Proto Schema Extension and Code Generation

**As a** developer, **I want** `Session` and `ListSessionsResponse` to carry memory fields, **so that** the frontend can display per-session and system-wide memory data from the existing poll cycle.

**Acceptance Criteria**:
- `proto/session/v1/types.proto` has `memory_rss_mb = 55` and `estimated_savings_mb = 56` on `Session`
- `proto/session/v1/session.proto` has `system_memory_pct = 2` on `ListSessionsResponse`
- `make generate-proto` runs without errors
- Generated Go and TypeScript bindings are committed together with proto changes
- Existing callers see zero values (NFR-4)

**Files**:
- `proto/session/v1/types.proto` (modified)
- `proto/session/v1/session.proto` (modified)
- `session/gen/session/v1/*.go` (auto-generated, committed)
- `web-app/src/gen/session/v1/*_pb.ts` (auto-generated, committed)

##### Task 1.3.1a: Add proto fields (~3 min)

- In `proto/session/v1/types.proto`, after `sub_status = 54;` (line ~178), add:
  ```protobuf
  // Per-session memory footprint in MB. Zero when hibernated or unknown.
  // Populated by the server from a 30-second RSS cache; approximate (RSS overcount is acceptable).
  int32 memory_rss_mb = 55;

  // Estimated RAM freed if this session is hibernated. Equals memory_rss_mb for Active sessions.
  // Zero for Hibernated sessions or when memory measurement is unavailable.
  int32 estimated_savings_mb = 56;
  ```
- In `proto/session/v1/session.proto`, in `ListSessionsResponse` after `sessions = 1;`, add:
  ```protobuf
  // Current system memory usage percentage (0.0–100.0). Populated at list time.
  // Zero when measurement is unavailable (e.g., macOS). Absence means below threshold.
  float system_memory_pct = 2;
  ```
- Files: `proto/session/v1/types.proto`, `proto/session/v1/session.proto`

##### Task 1.3.1b: Run make generate-proto (~2 min)

- Run `make generate-proto`
- Verify `session/gen/session/v1/` Go files updated (check `memory_rss_mb` field present)
- Verify `web-app/src/gen/session/v1/types_pb.ts` updated (check `memoryRssMb` field present)
- Commit ALL changed files under `session/gen/` and `web-app/src/gen/` together with the proto changes
- **This task is a gate**: no frontend tasks start until this is complete
- Files: `session/gen/session/v1/` (all regenerated), `web-app/src/gen/session/v1/` (all regenerated)

#### Story 1.3.2: Populate Memory Fields in API Layer

**As a** frontend developer, **I want** `ListSessions` to return populated memory fields, **so that** session cards can display real RSS values without an extra RPC.

**Acceptance Criteria**:
- `adapters.InstanceToProto` sets `MemoryRssMb` and `EstimatedSavingsMb` (from the shared memory cache)
- Hibernated instances always return 0 for both fields regardless of cache (Pitfall 5 mitigation)
- `ListSessionsResponse.SystemMemoryPct` is populated from `mem.VirtualMemory()` at list time
- Unit test for `InstanceToProto` covers hibernated=0, active=cached-value cases

**Files**:
- `server/adapters/instance_adapter.go` (modified)
- `server/adapters/instance_adapter_test.go` (modified)
- `server/services/session_service.go` (modified — `ListSessions` populates `SystemMemoryPct`)
- `session/hibernation_sweeper.go` (modified — expose cache for read access)

##### Task 1.3.2a: Expose memory cache and wire InstanceToProto (~5 min)

- Add exported `MemoryCacheReader` interface in `session/` package (same package as `HibernationSweeper`, no circular dep):
  ```go
  // MemoryCacheReader is implemented by HibernationSweeper; injected into SessionService.
  type MemoryCacheReader interface {
      GetCachedRSSMB(uuid string) int64
  }
  ```
- Add exported method `(s *HibernationSweeper) GetCachedRSSMB(uuid string) int64` — delegates to `s.memCache`; returns 0 if missing/expired
- **Wiring in `server/server.go`**: construct `sweeper` first, then pass `sweeper` (as `MemoryCacheReader`) to `SessionService`. Dependency order: `sweeper` → `SessionService`. No circular dep — `session/` is already imported by `server/services/`.
- Add `memCache session.MemoryCacheReader` field to `SessionService`
- Add `InstanceToProtoWithMemory(inst *session.Instance, cache session.MemoryCacheReader) *sessionv1.Session` function in `server/adapters/`:
  - If `inst` is nil: return nil
  - Start from `InstanceToProto(inst)` result (call it internally)
  - If `inst.IsHibernated()`: set `MemoryRssMb = 0`, `EstimatedSavingsMb = 0`
  - Else if `cache != nil`: `rssMB = cache.GetCachedRSSMB(inst.UUID.String())`, set both fields to `int32(rssMB)`
  - Return the populated proto
- Update all callers of `InstanceToProto` in `session_service.go` to call `InstanceToProtoWithMemory(inst, s.memCache)` instead
- **All call sites of `NewHibernationSweeper`**: run `grep -rn "NewHibernationSweeper" .` before starting. Known sites: `server/server.go` line ~495. Any test files constructing `HibernationSweeper` directly must also be updated (pass `memory.NewGopsutilReader(nil)` or a `FakeReader`). The constructor signature change is a compile error — the build will catch all missed sites.
- Files: `session/hibernation_sweeper.go`, `server/adapters/instance_adapter.go`, `server/services/session_service.go`, `server/server.go`

##### Task 1.3.2b: Populate system_memory_pct in ListSessions and WatchSessions (~4 min)

- In `SessionService`, add a `memReader memory.Reader` field (injected from `server.go` — same `GopsutilReader` instance used by sweeper)
- In `ListSessions`, before building the response, call `s.memReader.SystemMemory(ctx)` to get `usedPct`
- Set `ListSessionsResponse.SystemMemoryPct = float32(usedPct)` on the response
- Handle error: if reader errors, `SystemMemoryPct = 0` (graceful degradation)
- **WatchSessions event stream**: when `SessionEvent` is published (e.g., on hibernation), the event payload carries a `Session` proto. In `WatchSessions` and anywhere `SessionUpdatedEvent` is built, use `InstanceToProtoWithMemory(inst, s.memCache)` so memory fields are non-zero in streamed events. This ensures the live UI doesn't show stale 0 values after the initial `ListSessions` load. Search for all sites that construct `SessionEvent` payloads in `session_service.go` and update them.
- Update `server/server.go` to inject `memReader` into `SessionService`
- Files: `server/services/session_service.go`, `server/server.go`

##### Task 1.3.2c: Adapter unit tests (~3 min)

- In `server/adapters/instance_adapter_test.go`:
  - Test `InstanceToProtoWithMemory` with hibernated instance → `MemoryRssMb = 0`, `EstimatedSavingsMb = 0`
  - Test with active instance + cache returning 42 → `MemoryRssMb = 42`, `EstimatedSavingsMb = 42`
  - Test with nil cache → both fields = 0
- Files: `server/adapters/instance_adapter_test.go`

---

## Phase 2: Frontend

> All Phase 2 tasks depend on Task 1.3.1b (generate-proto gate) being complete.

### Epic 2.1: Global Memory Pressure Indicator

**Goal**: Show a `"Memory: N%"` badge in the header when system memory exceeds the configured threshold (FR-5).

#### Story 2.1.1: SystemMemoryContext and Header Badge

**As a** user, **I want** to see a global memory pressure indicator in the header when RAM is critically high, **so that** I know to take action before the system auto-hibernates sessions.

**Acceptance Criteria**:
- `SystemMemoryContext` exposes `{systemMemoryPct: number, threshold: number, isUnderPressure: boolean}`
- `systemMemoryPct` is sourced from `ListSessionsResponse.systemMemoryPct` via `useSessionService`
- `MemoryNavBadge` appears in `Header.tsx` only when `isUnderPressure === true`
- Badge displays `"Memory: N%"` using `vars.color.warning` text and `vars.color.warningBg` background
- Badge uses vanilla-extract CSS (not inline styles)
- Badge does not appear when `systemMemoryPct < threshold`

**Files**:
- `web-app/src/lib/contexts/SystemMemoryContext.tsx` (new)
- `web-app/src/components/sessions/MemoryNavBadge.tsx` (new)
- `web-app/src/components/sessions/MemoryNavBadge.css.ts` (new)
- `web-app/src/components/layout/Header.tsx` (modified)
- `web-app/src/lib/hooks/useSessionService.ts` (modified — expose `systemMemoryPct`)

##### Task 2.1.1a: Create SystemMemoryContext (~3 min)

- Create `web-app/src/lib/contexts/SystemMemoryContext.tsx`
- Context interface:
  ```ts
  interface SystemMemoryContextValue {
    systemMemoryPct: number;     // 0–100; 0 means unavailable
    threshold: number;           // from config; default 85
    isUnderPressure: boolean;    // systemMemoryPct >= threshold && systemMemoryPct > 0
  }
  ```
- Provider receives `systemMemoryPct` as a prop (sourced from `useSessionService`)
- `threshold` is hardcoded to 85 in the context default (matches `ResourcePressureThreshold` default; NFR-3 says no new config keys)
- Export `useSystemMemory()` hook
- Wire `SystemMemoryProvider` into `web-app/src/app/Providers.tsx` (wrapping children, inside `SessionServiceContext`)
- Files: `web-app/src/lib/contexts/SystemMemoryContext.tsx`, `web-app/src/app/Providers.tsx`

##### Task 2.1.1b: Expose systemMemoryPct from useSessionService (~4 min)

- In `UseSessionServiceReturn` interface, add `systemMemoryPct: number`
- In `useSessionService`, extract `systemMemoryPct` from `ListSessionsResponse` when `listSessions` resolves:
  - The response proto has `systemMemoryPct` field (from generated bindings)
  - Store in `useState<number>(0)` local state, set alongside session dispatch
- **Staleness problem**: after initial `listSessions()`, the live UI is driven by `WatchSessions` events which carry per-`Session` objects (not the list-level `systemMemoryPct`). To avoid stale values, call `listSessions()` on a 30-second timer alongside the existing watch transport. This is acceptable because `systemMemoryPct` updates at the same 30-second cache TTL cadence. Add a `useEffect` with `setInterval(listSessions, 30_000)` that is cleared on unmount/stop. Alternatively, if `WatchSessions` events are extended to carry `systemMemoryPct` (backend change in Task 1.3.2b), read it from there. Use whichever is simpler given the watch event structure — document the choice during implementation.
- Return `systemMemoryPct` from the hook
- In `SessionServiceContext.tsx` (or wherever `useSessionService` is consumed), pass `systemMemoryPct` to `SystemMemoryProvider`
- Files: `web-app/src/lib/hooks/useSessionService.ts`, `web-app/src/lib/contexts/SessionServiceContext.tsx`

##### Task 2.1.1c: Create MemoryNavBadge and wire into Header (~4 min)

- Create `web-app/src/components/sessions/MemoryNavBadge.css.ts`:
  ```ts
  import { style } from '@vanilla-extract/css';
  import { vars } from '../../styles/theme.css';

  export const memoryBadge = style({
    background: vars.color.warningBg,
    color: vars.color.warning,
    // ... padding, borderRadius, fontSize matching other nav badges
  });
  ```
- Create `web-app/src/components/sessions/MemoryNavBadge.tsx`:
  ```tsx
  "use client";
  import { useSystemMemory } from "@/lib/contexts/SystemMemoryContext";
  import { memoryBadge } from "./MemoryNavBadge.css";

  export function MemoryNavBadge() {
    const { systemMemoryPct, isUnderPressure } = useSystemMemory();
    if (!isUnderPressure) return null;
    return (
      <span className={memoryBadge} data-testid="memory-pressure-badge"
            aria-label={`Memory pressure: ${Math.round(systemMemoryPct)}%`}>
        Memory: {Math.round(systemMemoryPct)}%
      </span>
    );
  }
  ```
- In `Header.tsx`, import `MemoryNavBadge` and add it alongside `ApprovalNavBadge` in the actions area (around line 122)
- Files: `web-app/src/components/sessions/MemoryNavBadge.css.ts`, `web-app/src/components/sessions/MemoryNavBadge.tsx`, `web-app/src/components/layout/Header.tsx`

---

### Epic 2.2: Per-Session Memory Display

**Goal**: Show per-session RSS on session cards and rows, and update the hibernate button tooltip to include savings estimate (FR-4).

#### Story 2.2.1: Memory Display in SessionCard, SessionRow, and Hibernate Tooltip

**As a** user, **I want** to see how much RAM each session is using, **so that** I can make informed decisions about which sessions to hibernate.

**Acceptance Criteria**:
- Session card shows `"~N MB"` adjacent to the status badge when `memoryRssMb > 0`
- Session row shows `"~N MB"` in the elapsed/metadata area when `memoryRssMb > 0`
- Hibernated sessions show `"–"` (not "0 MB" or a stale figure) per FR-4.3
- Hibernate overflow menu item reads `"Hibernate · saves ~N MB"` when `estimatedSavingsMb > 0` (FR-4.2)
- When `estimatedSavingsMb == 0`, hibernate button reads `"Hibernate"` (unchanged)
- No layout breakage on cards with long titles

**Files**:
- `web-app/src/components/sessions/SessionCard.tsx` (modified)
- `web-app/src/components/sessions/SessionCard.css.ts` (modified)
- `web-app/src/components/sessions/SessionRow.tsx` (modified)
- `web-app/src/components/sessions/SessionRow.css.ts` (modified)
- `web-app/src/components/sessions/SessionActionsOverflow.tsx` (modified)

##### Task 2.2.1a: Add memory display to SessionCard (~4 min)

- In `SessionCard.tsx`, the `session` prop already includes the full `Session` proto object with generated `memoryRssMb` and `estimatedSavingsMb` fields (available after generate-proto)
- Add a `MemoryBadge` inline component or helper:
  ```tsx
  function MemoryBadge({ session }: { session: Session }) {
    if (session.status === SessionStatus.HIBERNATED) return <span className={memoryText}>–</span>;
    if (!session.memoryRssMb || session.memoryRssMb === 0) return null;
    return <span className={memoryText}>~{session.memoryRssMb} MB</span>;
  }
  ```
- Place `<MemoryBadge session={session} />` adjacent to the status badge in the card's info row
- Add `memoryText` style to `SessionCard.css.ts`:
  ```ts
  export const memoryText = style({
    fontSize: vars.fontSize.xs,
    color: vars.color.textMuted,
    // no margin — relies on flex gap in parent
  });
  ```
- Files: `web-app/src/components/sessions/SessionCard.tsx`, `web-app/src/components/sessions/SessionCard.css.ts`

##### Task 2.2.1b: Add memory display to SessionRow (~3 min)

- In `SessionRow.tsx`, apply same `MemoryBadge` pattern adjacent to the elapsed time display
- Reuse or import the `memoryText` style from `SessionCard.css.ts`, or define a parallel one in `SessionRow.css.ts`
- Files: `web-app/src/components/sessions/SessionRow.tsx`, `web-app/src/components/sessions/SessionRow.css.ts`

##### Task 2.2.1c: Update hibernate tooltip in SessionActionsOverflow (~3 min)

- `SessionActionsOverflow` already receives `session` as a prop (with all fields)
- Update the hibernate menu item (line ~395–401) to include savings:
  ```tsx
  {isRunning && onHibernate && (
    <button role="menuitem" className={overflowMenuItem}
      onClick={(e) => { e.stopPropagation(); close(); onHibernate(); }}
      aria-label={`Hibernate session ${session.title}${session.estimatedSavingsMb > 0 ? ` · saves ~${session.estimatedSavingsMb} MB` : ''}`}
    >
      <span aria-hidden="true">❄️</span>{' '}
      {session.estimatedSavingsMb > 0
        ? `Hibernate · saves ~${session.estimatedSavingsMb} MB`
        : 'Hibernate'}
    </button>
  )}
  ```
- Files: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

---

### Epic 2.3: Memory Pressure Callout and Session Highlights

**Goal**: When memory is under pressure, show a dismissible callout with the top-3 recommended sessions to hibernate and an amber left-border highlight on candidate cards (FR-6).

#### Story 2.3.1: MemoryPressureCallout Component

**As a** user under memory pressure, **I want** a proactive callout recommending which sessions to hibernate and offering a bulk action, **so that** I can act quickly without manually identifying the worst offenders.

**Acceptance Criteria**:
- `MemoryPressureCallout` renders only when `isUnderPressure === true`
- Lists up to 3 sessions sorted by `estimatedSavingsMb` descending with title and savings shown
- "Hibernate all recommended" button triggers `onHibernate` for each listed session
- Per-session dismiss button removes that session from the list for the current browser session (uses `useState<Set<string>>` of dismissed UUIDs)
- Callout can be fully dismissed (all recommendations dismissed or "dismiss" button)
- Dismissed sessions do not re-appear within the same browser session (FR-6.4)
- Callout is rendered inline above the session list (not a global toast) so it has direct access to session callbacks

**Files**:
- `web-app/src/components/sessions/MemoryPressureCallout.tsx` (new)
- `web-app/src/components/sessions/MemoryPressureCallout.css.ts` (new)
- `web-app/src/components/sessions/SessionList.tsx` (modified — renders callout, passes hibernate callbacks)

##### Task 2.3.1a: Create MemoryPressureCallout component (~5 min)

- Create `web-app/src/components/sessions/MemoryPressureCallout.css.ts` using vanilla-extract:
  ```ts
  import { style } from '@vanilla-extract/css';
  import { vars } from '../../styles/theme.css';

  export const callout = style({
    borderLeft: `3px solid ${vars.color.warning}`,
    background: vars.color.warningBg,
    color: vars.color.warningText,
    padding: `${vars.space[3]} ${vars.space[4]}`,
    marginBottom: vars.space[3],
    borderRadius: vars.radii.sm,
  });
  export const recommendationList = style({ listStyle: 'none', padding: 0, margin: `${vars.space[2]} 0` });
  export const recommendationItem = style({ display: 'flex', alignItems: 'center', gap: vars.space[2], padding: `${vars.space[1]} 0` });
  export const bulkButton = style({ /* primary action button style */ });
  export const dismissButton = style({ /* ghost/small button style */ });
  ```
- Create `web-app/src/components/sessions/MemoryPressureCallout.tsx`:
  ```tsx
  interface MemoryPressureCalloutProps {
    sessions: Session[];           // all sessions (component filters and sorts)
    onHibernate: (id: string) => void;
    systemMemoryPct: number;
    threshold?: number;            // default 85
  }

  export function MemoryPressureCallout({ sessions, onHibernate, systemMemoryPct, threshold = 85 }: MemoryPressureCalloutProps) {
    const [dismissed, setDismissed] = useState<Set<string>>(new Set());
    const [allDismissed, setAllDismissed] = useState(false);

    if (systemMemoryPct < threshold || systemMemoryPct === 0 || allDismissed) return null;

    const candidates = sessions
      .filter(s => s.status === SessionStatus.ACTIVE && s.estimatedSavingsMb > 0)
      .filter(s => !dismissed.has(s.id))
      .sort((a, b) => b.estimatedSavingsMb - a.estimatedSavingsMb)
      .slice(0, 3);

    if (candidates.length === 0) return null;

    const handleHibernateAll = () => candidates.forEach(s => onHibernate(s.id));
    const handleDismiss = (id: string) => setDismissed(prev => new Set([...prev, id]));

    return (
      <div className={callout} role="alert" data-testid="memory-pressure-callout">
        <strong>Memory pressure: {Math.round(systemMemoryPct)}%</strong>
        <p>Consider hibernating these sessions to free RAM:</p>
        <ul className={recommendationList}>
          {candidates.map(s => (
            <li key={s.id} className={recommendationItem}>
              <span>{s.title} — saves ~{s.estimatedSavingsMb} MB</span>
              <button className={dismissButton} onClick={() => handleDismiss(s.id)} aria-label={`Dismiss recommendation for ${s.title}`}>✕</button>
              <button className={dismissButton} onClick={() => onHibernate(s.id)} aria-label={`Hibernate ${s.title}`}>Hibernate</button>
            </li>
          ))}
        </ul>
        <button className={bulkButton} onClick={handleHibernateAll}>Hibernate all recommended</button>
        <button className={dismissButton} onClick={() => setAllDismissed(true)} aria-label="Dismiss memory pressure callout">Dismiss</button>
      </div>
    );
  }
  ```
- Files: `web-app/src/components/sessions/MemoryPressureCallout.tsx`, `web-app/src/components/sessions/MemoryPressureCallout.css.ts`

##### Task 2.3.1b: Wire MemoryPressureCallout into SessionList (~3 min)

- In `SessionList.tsx`, import `MemoryPressureCallout` and `useSystemMemory`
- Above the session list render, add:
  ```tsx
  <MemoryPressureCallout
    sessions={sessions}
    onHibernate={(id) => hibernateSession(id)}
    systemMemoryPct={systemMemoryPct}
    threshold={85}
  />
  ```
- The `sessions` prop already available in `SessionList`; `hibernateSession` is already available
- `systemMemoryPct` comes from `useSystemMemory()` hook in this component
- Files: `web-app/src/components/sessions/SessionList.tsx`

##### Task 2.3.1c: Add pressure-highlight CSS variant to SessionCard (~3 min)

- In `SessionCard.css.ts`, add a new style (or recipe variant) for the amber left-border highlight:
  ```ts
  export const cardPressureHighlight = style({
    borderLeft: `3px solid ${vars.color.warning}`,
    background: vars.color.warningBg,
  });
  ```
- In `SessionCard.tsx`, consume `useSystemMemory()` to get `isUnderPressure`:
  - Apply `cardPressureHighlight` when `isUnderPressure && session.estimatedSavingsMb > 0`
  - Use `clsx(card(), isUnderPressure && session.estimatedSavingsMb > 0 && cardPressureHighlight)`
- Files: `web-app/src/components/sessions/SessionCard.css.ts`, `web-app/src/components/sessions/SessionCard.tsx`

---

## Phase 3: Integration and Quality

### Epic 3.1: Build Validation and Smoke Tests

**Goal**: Confirm `make quick-check` passes with all changes integrated.

#### Story 3.1.1: Build, Test, Lint

**Acceptance Criteria**:
- `make build` passes (protos already regenerated)
- `make test` passes (all existing hibernation tests + new sweeper/adapter tests)
- `make lint` passes (no new linting violations)
- Frontend: `cd web-app && npx jest --no-coverage` passes

**Files**: No new files.

##### Task 3.1.1a: Run make quick-check and fix any failures (~5 min)

- Run `make quick-check` (build + test + lint)
- Fix any compilation errors (likely: missing imports in `session_service.go`, unused variables)
- Run `cd web-app && npx jest --no-coverage` and fix any TypeScript type errors in new components
- Common issues to watch for:
  - `InstanceToProtoWithMemory` not called everywhere `InstanceToProto` was
  - `memory.Reader` import cycle if `sessionMemoryCache` is placed incorrectly
  - CSS token names: must use `vars.color.warning` / `vars.color.warningBg` / `vars.color.warningText` (not `statusWarning` — that token does not exist per architecture research)
  - `clsx` import needed in `SessionCard.tsx` if not already present
- Files: whichever files have errors

---

## Summary

### File Inventory

| File | Action |
|---|---|
| `session/memory/reader.go` | New |
| `session/memory/reader_test.go` | New |
| `session/hibernation_sweeper.go` | Modified |
| `session/hibernation_sweeper_test.go` | New/Modified |
| `proto/session/v1/types.proto` | Modified |
| `proto/session/v1/session.proto` | Modified |
| `session/gen/session/v1/*.go` | Auto-generated |
| `web-app/src/gen/session/v1/*_pb.ts` | Auto-generated |
| `server/adapters/instance_adapter.go` | Modified |
| `server/adapters/instance_adapter_test.go` | Modified |
| `server/services/session_service.go` | Modified |
| `server/server.go` | Modified |
| `web-app/src/lib/hooks/useSessionService.ts` | Modified |
| `web-app/src/lib/contexts/SystemMemoryContext.tsx` | New |
| `web-app/src/lib/contexts/SessionServiceContext.tsx` | Modified |
| `web-app/src/app/Providers.tsx` | Modified |
| `web-app/src/components/sessions/MemoryNavBadge.tsx` | New |
| `web-app/src/components/sessions/MemoryNavBadge.css.ts` | New |
| `web-app/src/components/layout/Header.tsx` | Modified |
| `web-app/src/components/sessions/SessionCard.tsx` | Modified |
| `web-app/src/components/sessions/SessionCard.css.ts` | Modified |
| `web-app/src/components/sessions/SessionRow.tsx` | Modified |
| `web-app/src/components/sessions/SessionRow.css.ts` | Modified |
| `web-app/src/components/sessions/SessionActionsOverflow.tsx` | Modified |
| `web-app/src/components/sessions/MemoryPressureCallout.tsx` | New |
| `web-app/src/components/sessions/MemoryPressureCallout.css.ts` | New |
| `web-app/src/components/sessions/SessionList.tsx` | Modified |

### Proto Field Numbers (Final)

| Field | Proto file | Field number | Rationale |
|---|---|---|---|
| `Session.memory_rss_mb` | `types.proto` | **55** | Next after `sub_status = 54` |
| `Session.estimated_savings_mb` | `types.proto` | **56** | Next after `memory_rss_mb` |
| `ListSessionsResponse.system_memory_pct` | `session.proto` | **2** | Next after `sessions = 1` |

> **Note**: Research docs stated fields 52-54 were free, but the current codebase already uses `cdp_state = 52`, `creation_progress = 53`, `sub_status = 54`. Fields 55 and 56 are correct.

### Counts

- **Epics**: 7 (1.1, 1.2, 1.3, 2.1, 2.2, 2.3, 3.1)
- **Stories**: 8
- **Tasks**: 19

### Key Design Decisions

1. **`session/memory/` package** (not `procinfo/`): clean interface boundary for test injection
2. **Sweeper constructor updated** (`NewHibernationSweeper` gains `memory.Reader` param): injectable for tests
3. **`MemoryCacheReader` interface** injected into `SessionService`: avoids circular dependency between `server/services/` and `session/` cache
4. **Proto fields 55/56** (not 52/53 as in research): corrects the research error; fields 52-54 are already used
5. **macOS sentinel = 0.0** from `SystemMemory()`: disables pressure hibernation without triggering it
6. **`InstanceToProtoWithMemory`** instead of modifying existing `InstanceToProto` signature: preserves backward compatibility for callers that don't have a cache
7. **`SystemMemoryContext`** for global badge access: avoids prop-drilling through layout hierarchy
8. **`MemoryPressureCallout` is inline in `SessionList`**, not a global toast: direct access to session data and hibernate callbacks
9. **CSS token**: `vars.color.warning` / `vars.color.warningBg` / `vars.color.warningText` — NOT `statusWarning` (does not exist in theme contract)
