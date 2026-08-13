# Validation Plan: memory-pressure-ux

**Date**: 2026-05-21

---

## Requirement → Test Mapping

### FR-1: Memory Measurement (Backend)

| Requirement | Test File | Test Name | Type | Scenario | Mock Strategy | Pass/Fail Signal |
|---|---|---|---|---|---|---|
| FR-1.1: System memory via `/proc/meminfo` | `session/memory/reader_test.go` | `TestGopsutilReader_SystemMemory_should_returnUsedPct_When_VirtualMemorySucceeds` | Unit (happy) | Returns `UsedPct` from `mem.VirtualMemory()` | Inject `FakeReader{SystemPct: 72.5}`; assert `stats.UsedPct == 72.5` | Returned `UsedPct` matches configured value |
| FR-1.1: System memory error → zero | `session/memory/reader_test.go` | `TestGopsutilReader_SystemMemory_should_returnZero_When_VirtualMemoryErrors` | Unit (error) | `VirtualMemory()` fails → returns `SystemStats{}` (zero, disables pressure) | Inject `FakeReader{SystemPct: 0, SystemErr: errors.New("unavailable")}`; assert `stats.UsedPct == 0.0` | UsedPct is exactly 0.0; no panic |
| FR-1.2: Per-session RSS via process walk | `session/memory/reader_test.go` | `TestGopsutilReader_ProcessMemory_should_sumRSS_When_ValidPIDs` | Unit (happy) | Walks child PIDs to depth 3, sums RSS in MB | `FakeReader{ProcRSSMB: 42}`; assert `stats.RSSMB == 42` | RSSMB equals sum of configured fake processes |
| FR-1.2: Process memory error path | `session/memory/reader_test.go` | `TestGopsutilReader_ProcessMemory_should_skipDeadPID_When_ProcessNoLongerExists` | Unit (error) | Individual PID exits mid-walk; loop continues without returning error | Inject PID list where one PID errors; assert result excludes that PID but includes others | No error returned; RSSMB equals sum of surviving PIDs |
| FR-1.2: Depth-3 cap and 50-PID cap | `session/memory/reader_test.go` | `TestGopsutilReader_ProcessMemory_should_capAt50PIDs_When_DeepProcessTree` | Unit | Process tree deeper than 3 / wider than 50 is capped without infinite loop | Fake child resolver returning 60 children at depth 1; assert exactly 50 PIDs collected | Walk terminates; total PIDs collected ≤ 50 |
| FR-1.3: PID resolution via tmux pane | `session/memory/reader_test.go` | `TestTmuxPIDResolver_should_returnPIDs_When_TmuxSucceeds` | Unit (happy) | Parses `tmux list-panes` output lines as `int32` PIDs | Inject a `PIDResolver` func that returns `[]int32{1234, 5678}`; verify downstream call uses those PIDs | PIDs passed to `ProcessMemory` match injected list |
| FR-1.3: PID resolver returns empty on tmux not found | `session/memory/reader_test.go` | `TestTmuxPIDResolver_should_returnEmptySlice_When_TmuxSessionNotFound` | Unit (error) | `tmux` exits non-zero; resolver returns empty slice, not error | Mock exec that returns exit code 1; assert empty `[]int32{}` returned, no error | Empty slice returned, no error, caller sees 0 MB |
| FR-1.4: 30s TTL cache | `session/hibernation_sweeper_test.go` | `TestSessionMemoryCache_should_returnCachedValue_When_WithinTTL` | Unit (happy) | Second `GetOrFetch` within 30s returns cached value without calling `fetchFn` | Call `GetOrFetch` twice with same UUID; count `fetchFn` invocations | `fetchFn` called exactly once |
| FR-1.4: Cache expires after 30s | `session/hibernation_sweeper_test.go` | `TestSessionMemoryCache_should_callFetchFn_When_TTLExpired` | Unit | After TTL passes, next `GetOrFetch` re-fetches | Inject fake clock; advance past 30s; call `GetOrFetch`; count `fetchFn` invocations | `fetchFn` called twice (initial + after expiry) |
| FR-1.4: Cache invalidated on hibernate | `session/hibernation_sweeper_test.go` | `TestSessionMemoryCache_should_callFetchFn_After_InvalidateCalled` | Unit | `Invalidate(uuid)` causes next `GetOrFetch` to re-fetch regardless of TTL | Call `GetOrFetch`, then `Invalidate`, then `GetOrFetch`; count `fetchFn` | `fetchFn` called twice |
| FR-1.4: Integration — cache shared between sweeper and ListSessions | `server/adapters/instance_adapter_test.go` | `TestInstanceToProtoWithMemory_should_returnCachedRSS_When_ActiveSession` | Integration | `InstanceToProtoWithMemory` reads from injected `MemoryCacheReader` | Stub `MemoryCacheReader.GetCachedRSSMB` returning 42; assert proto `MemoryRssMb == 42` | Proto field matches cache value |

---

### FR-2: Resource-Pressure Hibernation (Backend)

| Requirement | Test File | Test Name | Type | Scenario | Mock Strategy | Pass/Fail Signal |
|---|---|---|---|---|---|---|
| FR-2.1: Sweeper checks memory pressure each tick | `session/hibernation_sweeper_test.go` | `TestSweeper_should_callSystemMemory_When_SweepRuns` | Unit (happy) | `sweep()` calls `memReader.SystemMemory()` once per invocation | `FakeReader{SystemPct: 80}`; invoke `sweep()` once; assert `SystemMemory` call count = 1 | `SystemMemory` called exactly once per sweep |
| FR-2.2: Hibernates longest-idle session when above threshold | `session/hibernation_sweeper_test.go` | `TestSweeper_sweepResourcePressure_should_hibernateLongestIdleSession_When_AboveThreshold` | Unit (happy) | Two idle sessions; only the longer-idle one is hibernated per tick | `FakeReader{SystemPct: 90}`; threshold=85; session A idle 20m, B idle 10m; assert only A hibernated | A.Hibernate called once; B.Hibernate not called |
| FR-2.2: No hibernation below threshold | `session/hibernation_sweeper_test.go` | `TestSweeper_sweepResourcePressure_should_notHibernate_When_BelowThreshold` | Unit | `SystemPct=80`, threshold=85 → no pressure hibernate | `FakeReader{SystemPct: 80}`; assert no Hibernate calls | Zero hibernations |
| FR-2.2: Zero sentinel skips pressure block | `session/hibernation_sweeper_test.go` | `TestSweeper_sweepResourcePressure_should_skip_When_SystemPctIsZero` | Unit | macOS / unavailable → `UsedPct=0`; pressure logic skipped entirely | `FakeReader{SystemPct: 0}`; threshold=85; assert no Hibernate calls | Zero hibernations; no error logged |
| FR-2.2: No eligible session (all active < 5min idle) | `session/hibernation_sweeper_test.go` | `TestSweeper_sweepResourcePressure_should_notHibernate_When_NoEligibleIdleSessions` | Unit (error) | All active sessions had output within last 5 minutes | `FakeReader{SystemPct: 90}`; all sessions with `LastMeaningfulOutput` 2min ago; assert zero hibernations | Zero hibernations |
| FR-2.2: One-at-a-time per tick | `session/hibernation_sweeper_test.go` | `TestSweeper_sweepResourcePressure_should_hibernateOnlyOne_When_MultipleEligible` | Unit | Three eligible sessions; exactly one hibernated per `sweepResourcePressure` call | `FakeReader{SystemPct: 90}`; 3 idle sessions; assert Hibernate called exactly once | Exactly 1 Hibernate call |
| FR-2.2: Threshold=0 disables pressure hibernation | `session/hibernation_sweeper_test.go` | `TestSweeper_sweepResourcePressure_should_beSkipped_When_ThresholdIsZero` | Unit | `ResourcePressureThreshold=0` gates the entire pressure block | Set threshold=0; `FakeReader{SystemPct: 90}`; assert `sweepResourcePressure` never entered | `SystemMemory` not called; zero hibernations |
| FR-2.3: Reason field set to `"resource_pressure"` | `session/hibernation_sweeper_test.go` | `TestSweeper_sweepResourcePressure_should_setReasonResourcePressure_When_AutoHibernating` | Unit | Checkpoint reason field = `"resource_pressure"` after auto-hibernate | Capture `SetHibernateReason` arg; assert equals `"resource_pressure"` | Reason arg = `"resource_pressure"` |
| FR-2.3: Log line contains `reason=resource_pressure` | `session/hibernation_sweeper_test.go` | `TestSweeper_sweepResourcePressure_should_logReasonResourcePressure_When_HibernatingForPressure` | Unit | Log output includes `reason=resource_pressure` and session name | Capture log output; assert contains both strings | Log line contains `reason=resource_pressure` |
| FR-2.4: Grace period — no hibernation within 5min of output | `session/hibernation_sweeper_test.go` | `TestSweeper_sweepResourcePressure_should_respectGracePeriod_When_SessionHadRecentOutput` | Unit | Session with `LastMeaningfulOutput` 4min 59s ago is skipped | `FakeReader{SystemPct: 92}`; session idle 4m59s; assert no Hibernate call | Zero hibernations |

---

### FR-3: Memory API (Backend → Frontend)

| Requirement | Test File | Test Name | Type | Scenario | Mock Strategy | Pass/Fail Signal |
|---|---|---|---|---|---|---|
| FR-3.1: `memory_rss_mb` populated in ListSessions | `server/adapters/instance_adapter_test.go` | `TestInstanceToProtoWithMemory_should_setMemoryRssMb_When_CacheHasValue` | Unit (happy) | Active session + cache returning 42 MB → proto field = 42 | Fake `MemoryCacheReader` returning 42; active instance; assert `MemoryRssMb == 42` | `MemoryRssMb == 42` |
| FR-3.1: `estimated_savings_mb` populated for active session | `server/adapters/instance_adapter_test.go` | `TestInstanceToProtoWithMemory_should_setEstimatedSavingsMb_When_ActiveSession` | Unit (happy) | For active session, `EstimatedSavingsMb == MemoryRssMb` | Fake cache returning 42; assert `EstimatedSavingsMb == 42` | Both fields equal |
| FR-3.1: Hibernated session always returns 0 | `server/adapters/instance_adapter_test.go` | `TestInstanceToProtoWithMemory_should_returnZeroFields_When_Hibernated` | Unit | Hibernated instance → both memory fields = 0 regardless of cache | Fake cache returning 42; hibernated instance; assert both = 0 | Both fields = 0 |
| FR-3.1: Nil cache → both fields = 0 | `server/adapters/instance_adapter_test.go` | `TestInstanceToProtoWithMemory_should_returnZeroFields_When_NilCache` | Unit (error) | `nil` cache → graceful zero | Pass `nil` as cache; assert both fields = 0 | No panic; both = 0 |
| FR-3.1: `system_memory_pct` in ListSessionsResponse | `server/services/session_service_test.go` | `TestListSessions_should_populateSystemMemoryPct_When_MemReaderSucceeds` | Integration | `ListSessions` response carries `SystemMemoryPct` from reader | Inject `FakeReader{SystemPct: 87.3}`; call `ListSessions`; assert `response.SystemMemoryPct ≈ 87.3` | Response field matches reader value |
| FR-3.1: `system_memory_pct` graceful degradation | `server/services/session_service_test.go` | `TestListSessions_should_setSystemMemoryPctToZero_When_MemReaderErrors` | Integration (error) | Reader error → `SystemMemoryPct = 0` | Inject `FakeReader{SystemErr: errors.New("fail")}`; assert `SystemMemoryPct == 0` | Field = 0; no error returned to caller |
| FR-3.2: Memory fields re-fetched on each poll | `server/services/session_service_test.go` | `TestListSessions_should_callSystemMemoryOnEachInvocation_When_Polled` | Integration | Each call to `ListSessions` calls `SystemMemory` | Call `ListSessions` twice; assert `FakeReader.SystemMemoryCalls == 2` | Call count = invocation count |
| FR-3.1: WatchSessions events carry memory fields | `server/services/session_service_test.go` | `TestWatchSessions_should_populateMemoryFields_When_SessionUpdated` | Integration | `SessionUpdatedEvent` uses `InstanceToProtoWithMemory`, not bare `InstanceToProto` | Inject `FakeReader`; trigger a session update; capture event payload; assert `MemoryRssMb > 0` | Event payload has non-zero `MemoryRssMb` |

---

### FR-4: Session Card / Row UI

| Requirement | Test File | Test Name | Type | Scenario | Mock Strategy | Pass/Fail Signal |
|---|---|---|---|---|---|---|
| FR-4.1: Memory shown on card when > 0 | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard > should display memory badge when memoryRssMb is positive` | Unit (happy) | Card renders `"~42 MB"` adjacent to status badge | Render with `session.memoryRssMb = 42`; `getByText(/~42 MB/)` | Element found in rendered output |
| FR-4.1: Memory hidden when 0 | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard > should not display memory badge when memoryRssMb is zero` | Unit | No memory badge when field = 0 | Render with `session.memoryRssMb = 0`; assert no `/MB/` text | No MB text in DOM |
| FR-4.2: Hibernate tooltip with savings | `web-app/src/components/sessions/SessionActionsOverflow.test.tsx` | `SessionActionsOverflow > should show savings in hibernate button when estimatedSavingsMb is positive` | Unit (happy) | Hibernate button text = `"Hibernate · saves ~42 MB"` | Render with `session.estimatedSavingsMb = 42`; `getByText(/Hibernate · saves ~42 MB/)` | Text found |
| FR-4.2: Hibernate tooltip without savings | `web-app/src/components/sessions/SessionActionsOverflow.test.tsx` | `SessionActionsOverflow > should show plain Hibernate text when estimatedSavingsMb is zero` | Unit | When savings = 0, button reads plain `"Hibernate"` | Render with `session.estimatedSavingsMb = 0`; `getByText(/^Hibernate$/)` | Plain text; no savings text |
| FR-4.3: Hibernated session shows `"–"` not stale MB | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard > should show dash for hibernated session memory display` | Unit | Status=HIBERNATED → show `"–"` (not "0 MB" or any cached value) | Render with `session.status = SessionStatus.HIBERNATED`; `getByText("–")` | `"–"` rendered; no "MB" in memory area |
| FR-4.1: Memory shown on row when > 0 | `web-app/src/components/sessions/SessionRow.test.tsx` | `SessionRow > should display memory badge when memoryRssMb is positive` | Unit (happy) | Row renders `"~42 MB"` | Render with `session.memoryRssMb = 42`; `getByText(/~42 MB/)` | Element found |
| FR-4.3: Hibernated row shows `"–"` | `web-app/src/components/sessions/SessionRow.test.tsx` | `SessionRow > should show dash for hibernated session in memory area` | Unit | Status=HIBERNATED → `"–"` | Render with hibernated session; assert `"–"` present | `"–"` rendered |

---

### FR-5: Global Memory Pressure Indicator

| Requirement | Test File | Test Name | Type | Scenario | Mock Strategy | Pass/Fail Signal |
|---|---|---|---|---|---|---|
| FR-5.1: Badge appears when at or above threshold | `web-app/src/components/sessions/MemoryNavBadge.test.tsx` | `MemoryNavBadge > should render badge when systemMemoryPct is at threshold` | Unit (happy) | `systemMemoryPct = 85`, `threshold = 85` → badge visible | Wrap with `SystemMemoryContext` providing `{systemMemoryPct: 85, threshold: 85, isUnderPressure: true}`; `getByTestId("memory-pressure-badge")` | Badge element present |
| FR-5.1: Badge appears above threshold | `web-app/src/components/sessions/MemoryNavBadge.test.tsx` | `MemoryNavBadge > should render badge when systemMemoryPct exceeds threshold` | Unit (happy) | `systemMemoryPct = 92` → badge visible | Same pattern with 92%; badge found | Badge element present |
| FR-5.2: Badge shows correct text and color | `web-app/src/components/sessions/MemoryNavBadge.test.tsx` | `MemoryNavBadge > should display Memory N% text when under pressure` | Unit | Badge text = `"Memory: 87%"` | `systemMemoryPct = 87.3`; `getByText(/Memory: 87%/)` | Text matches; className includes warning token |
| FR-5.3: Badge hidden below threshold | `web-app/src/components/sessions/MemoryNavBadge.test.tsx` | `MemoryNavBadge > should not render badge when systemMemoryPct is below threshold` | Unit | `systemMemoryPct = 84`, threshold=85 → badge absent | `isUnderPressure: false`; `queryByTestId("memory-pressure-badge")` = null | Null — not in DOM |
| FR-5.3: Badge hidden when systemMemoryPct is zero | `web-app/src/components/sessions/MemoryNavBadge.test.tsx` | `MemoryNavBadge > should not render badge when systemMemoryPct is zero` | Unit | Zero = unavailable → no badge | `{systemMemoryPct: 0, isUnderPressure: false}`; query returns null | Null — not in DOM |
| FR-5.1: SystemMemoryContext isUnderPressure calculation | `web-app/src/lib/contexts/SystemMemoryContext.test.tsx` | `SystemMemoryContext > isUnderPressure should be true when pct >= threshold and pct > 0` | Unit (happy) | Context correctly derives `isUnderPressure` | Render provider with `systemMemoryPct=90`; consume hook; assert `isUnderPressure === true` | Derived flag = true |
| FR-5.1: Context isUnderPressure false for zero | `web-app/src/lib/contexts/SystemMemoryContext.test.tsx` | `SystemMemoryContext > isUnderPressure should be false when systemMemoryPct is zero` | Unit (error) | Sentinel 0 → not under pressure | Provider with `systemMemoryPct=0`; assert `isUnderPressure === false` | Flag = false |
| FR-5.2: Badge uses warning color token (not inline style) | `web-app/src/components/sessions/MemoryNavBadge.test.tsx` | `MemoryNavBadge > should use vanilla-extract className not inline color style` | Unit | No `style` attribute with color on badge element | Render and inspect; assert `element.style.color` is empty / not set | `style` attribute absent or empty; class is set |

---

### FR-6: Proactive Pause Recommendations

| Requirement | Test File | Test Name | Type | Scenario | Mock Strategy | Pass/Fail Signal |
|---|---|---|---|---|---|---|
| FR-6.1: Callout appears when under pressure | `web-app/src/components/sessions/MemoryPressureCallout.test.tsx` | `MemoryPressureCallout > should render callout when systemMemoryPct exceeds threshold` | Unit (happy) | `systemMemoryPct = 90`, eligible sessions → callout visible | Render with pressure context + sessions having `estimatedSavingsMb > 0`; `getByTestId("memory-pressure-callout")` | Callout element found |
| FR-6.1: Callout hidden below threshold | `web-app/src/components/sessions/MemoryPressureCallout.test.tsx` | `MemoryPressureCallout > should not render callout when below threshold` | Unit | `systemMemoryPct = 80` → no callout | Render with 80%; `queryByTestId("memory-pressure-callout")` = null | Null |
| FR-6.1: Top-3 sorted by savings descending | `web-app/src/components/sessions/MemoryPressureCallout.test.tsx` | `MemoryPressureCallout > should display top 3 sessions sorted by estimatedSavingsMb descending` | Unit (happy) | 5 sessions → only top-3 shown, in descending order | Render with 5 sessions having savings [10, 50, 30, 20, 40]; assert 3 items; first = 50 MB | 3 items; correct order |
| FR-6.1: No callout when no eligible sessions | `web-app/src/components/sessions/MemoryPressureCallout.test.tsx` | `MemoryPressureCallout > should not render callout when no sessions have estimatedSavingsMb > 0` | Unit (error) | Under pressure but all sessions `estimatedSavingsMb = 0` → no callout | Render with pressure but sessions with savings=0; query returns null | Null |
| FR-6.2: Each recommendation shows title and savings | `web-app/src/components/sessions/MemoryPressureCallout.test.tsx` | `MemoryPressureCallout > should display session title and savings estimate in each recommendation` | Unit | Each list item shows title + `"saves ~N MB"` | Render with session title "Work Session", savings 42; `getByText(/Work Session — saves ~42 MB/)` | Text found for each item |
| FR-6.3: Bulk action hibernates all listed | `web-app/src/components/sessions/MemoryPressureCallout.test.tsx` | `MemoryPressureCallout > should call onHibernate for each candidate when Hibernate all clicked` | Unit (happy) | "Hibernate all recommended" triggers `onHibernate` for each of top-3 | Mock `onHibernate`; click button; assert called 3 times with correct IDs | `onHibernate` called N times matching candidates |
| FR-6.4: Per-session dismiss removes from list | `web-app/src/components/sessions/MemoryPressureCallout.test.tsx` | `MemoryPressureCallout > should remove session from recommendations when dismiss button clicked` | Unit | Dismiss one session → it disappears from list, others remain | Click per-session dismiss; assert dismissed session no longer in DOM | Item removed; others remain |
| FR-6.4: Dismissed session does not re-appear | `web-app/src/components/sessions/MemoryPressureCallout.test.tsx` | `MemoryPressureCallout > should not re-show dismissed session after list re-render` | Unit | Re-render after dismiss → still absent | Dismiss; force re-render (update props); assert dismissed item still absent | Persists across re-render within same session |
| FR-6.4: Full callout dismissible | `web-app/src/components/sessions/MemoryPressureCallout.test.tsx` | `MemoryPressureCallout > should hide entire callout when global Dismiss clicked` | Unit | "Dismiss" button → entire callout gone | Click global dismiss; `queryByTestId("memory-pressure-callout")` = null | Null |
| FR-6.5: Amber highlight on candidate cards | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard > should apply pressure highlight class when under pressure and estimatedSavingsMb > 0` | Unit (happy) | Under pressure + savings > 0 → amber border class applied | Wrap with `isUnderPressure: true`; session `estimatedSavingsMb = 42`; assert card has `cardPressureHighlight` class | Class present |
| FR-6.5: No highlight below threshold | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard > should not apply pressure highlight class when not under pressure` | Unit | Not under pressure → no amber border | `isUnderPressure: false`; assert class absent | Class absent |
| FR-6.5: No highlight when savings = 0 | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard > should not apply pressure highlight class when estimatedSavingsMb is zero` | Unit | Under pressure but savings = 0 → no highlight | `isUnderPressure: true`, `estimatedSavingsMb = 0`; assert class absent | Class absent |

---

## Test Stack

- **Unit (Go)**: `testing` package, `testify/assert`, `FakeReader` struct for `memory.Reader` injection; fake `MemoryCacheReader` for adapter tests; fake instance constructors for sweeper tests. No real tmux, no real `/proc`.
- **Unit (TypeScript/React)**: Jest + React Testing Library; `SystemMemoryContext` test providers; plain prop injection for all component tests.
- **Integration (Go)**: `httptest` + real `SessionService` wired with `FakeReader` and in-memory storage; verifies `ListSessions` RPC response fields end-to-end within the Go process.
- **E2E**: None required for this feature (all acceptance criteria are verifiable at unit/integration level). Existing E2E suite covers session lifecycle; adding memory display assertions would require a real `/proc` environment.

---

## Coverage Targets

- Unit test coverage: ≥ 80% line coverage for `session/memory/` and `server/adapters/` packages
- All public service methods affected by this feature: happy path + error paths covered
- All external integrations (`memory.Reader` → `/proc`/`gopsutil`): unit-mocked via `FakeReader`; integration test exercises the full `ListSessions` RPC path
- All frontend components: happy path + hidden/absent state + dismiss/interaction flows

---

## Test Count Summary

| Category | Count |
|---|---|
| Go unit tests | 28 |
| Go integration tests | 4 |
| TypeScript/React unit tests | 30 |
| **Total** | **62** |

---

## Requirement Coverage

| FR | Tests Covering It | Coverage |
|---|---|---|
| FR-1.1 | 2 unit + 1 integration | Full |
| FR-1.2 | 3 unit | Full |
| FR-1.3 | 2 unit | Full |
| FR-1.4 | 3 unit + 1 integration | Full |
| FR-2.1 | 1 unit | Full |
| FR-2.2 | 6 unit | Full |
| FR-2.3 | 2 unit | Full |
| FR-2.4 | 1 unit | Full |
| FR-3.1 | 5 unit + 3 integration | Full |
| FR-3.2 | 1 integration | Full |
| FR-4.1 | 4 unit | Full |
| FR-4.2 | 2 unit | Full |
| FR-4.3 | 2 unit | Full |
| FR-5.1 | 4 unit | Full |
| FR-5.2 | 2 unit | Full |
| FR-5.3 | 2 unit | Full |
| FR-6.1 | 4 unit | Full |
| FR-6.2 | 1 unit | Full |
| FR-6.3 | 1 unit | Full |
| FR-6.4 | 3 unit | Full |
| FR-6.5 | 3 unit | Full |

**Requirements covered: 21/21 (100%)**
