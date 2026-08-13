# Pitfalls Research: memory-pressure-ux

## Pitfall 1: tmux pane_pid Returns Shell PID, Not AI Process PID

`tmux list-panes -t <session> -F '#{pane_pid}'` returns the PID of the shell launched
by tmux (e.g., `/bin/bash`), not the claude/aider process. The AI process is a child
of the shell.

**Risk**: Measuring RSS of the shell PID alone gives ~4 MB (near-zero). The real AI
process (node.js + claude binary) is a grandchild or deeper descendant.

**Mitigation options**:

1. **gopsutil `Children()`**: `proc.Children()` returns immediate children. Recursing
   to depth 2–3 captures typical shell → AI → subprocess chains. Depth limit prevents
   infinite loops if a process re-parents.

2. **`/proc/<pid>/task/<tid>/children`** (Linux-only): More precise, reads the kernel's
   task list directly. Example:
   ```
   /proc/<shell_pid>/task/<shell_pid>/children → "<ai_pid>"
   /proc/<ai_pid>/task/<ai_pid>/children → "<tool_pids...>"
   ```
   Requires Linux build tag.

3. **Safe approach**: Use gopsutil `proc.Children()` (cross-platform) + aggregate RSS
   of shell + all children + grandchildren up to depth 3. Cap at 50 PIDs per session
   to avoid runaway traversal.

**Chosen approach for this feature**: gopsutil `Children()` with depth-3 cap. One
gotcha: `Children()` returns `([]*process.Process, error)` — an error when no children
exist is benign. Treat it as empty slice.

---

## Pitfall 2: RSS Overcount Due to Shared Memory

Summing RSS across a process tree double-counts shared library pages. A node.js AI
process and its spawned subprocesses both map libc.so — counting both inflates the total.

**Magnitude**: For claude, realistic overcount is 20–60 MB (shared node.js runtime, V8
heap mapped into workers). For a process with 300 MB RSS, this could show 380 MB.

**Mitigations**:
1. **PSS (Proportional Set Size)**: Available via `/proc/<pid>/smaps_rollup` on Linux.
   PSS divides shared pages proportionally. More accurate but requires Linux-only code.
2. **Accept RSS overcount**: For UX purposes (showing "~N MB" with a tilde), slight
   overcount is acceptable and actually conservative (shows users the worst case).

**Decision**: Accept RSS overcount for the first implementation. Document this in code
comments. PSS can be a follow-up optimization. The `"~"` prefix in the UI ("saves ~42 MB")
signals approximation to users.

---

## Pitfall 3: Auto-Hibernating a Session the User Is Actively Viewing

The sweeper hibernates the longest-idle session by `TimeSinceLastMeaningfulOutput`. A
session could be idle in the terminal but the user has it open in the UI.

**Current sweeper behavior**: The sweeper checks `TimeSinceLastMeaningfulOutput` — if
no output for `IdleTimeoutMinutes`, it hibernates. The existing idle-timeout hibernation
has this same risk.

**FR-2.4 guard**: "No session that has seen meaningful output in the last 5 minutes must
be auto-hibernated for resource pressure." This is enforced by:
```go
if inst.TimeSinceLastMeaningfulOutput(inst.CreatedAt) < 5*time.Minute {
    continue // skip — recently active
}
```

**Additional mitigation**: `LastViewed` on Instance records when the user last viewed a
session. Consider adding a guard: if `time.Since(inst.LastViewed) < 5*time.Minute`,
skip. However, FR-2.4 only specifies meaningful output, not view time — adding LastViewed
check would be a nice-to-have beyond requirements.

**Race condition**: The sweeper reads `LastMeaningfulOutput` without holding `stateMutex`.
This is the same race as the existing sweep() (line 91 in hibernation_sweeper.go). Since
`LastMeaningfulOutput` is a `time.Time` value (8 bytes on 64-bit), concurrent writes are
effectively atomic in practice on amd64/arm64. The existing code accepts this race.
**No change needed** — consistent with existing sweeper behavior.

---

## Pitfall 4: Proto Backward Compatibility (Generate Step)

Adding optional fields to `Session` (types.proto) and `ListSessionsResponse`
(session.proto) is backward-compatible per proto3 rules. Existing callers see zero values.

**Risk**: Forgetting to run `make generate-proto` (which runs `buf generate proto`).
The Makefile target regenerates:
- `session/gen/session/v1/*.go` (Go bindings)
- `web-app/src/gen/session/v1/*_pb.ts` (TypeScript bindings)

**Pitfall**: Committing proto changes without regenerating. The CI `make build` step
runs `buf lint proto` and `buf build proto` but does NOT run `buf generate proto`
automatically unless the stamp file is stale.

**Mitigation**: Always run `make generate-proto` after proto edits and commit ALL changed
files under `session/gen/` and `web-app/src/gen/` together.

**Field number allocation**: `Session` message uses fields up to 51 (`vnc_state`).
Use 52, 53 for `memory_rss_mb` and `estimated_savings_mb`. `ListSessionsResponse`
uses field 1; use field 2 for `system_memory_pct`.

---

## Pitfall 5: Cache Invalidation on Hibernation

The 30-second TTL memory cache holds RSS for active sessions. If a session is hibernated
(manually or by sweeper), the cached RSS becomes stale — the UI would show "42 MB" for
a session that is now dead.

**Risk**: Cache not invalidated → next poll shows stale RSS → FR-4.3 violated
("Hibernated sessions must show '–' or '0 MB'").

**Mitigation**: The cache should be keyed by session UUID. The `HibernationSweeper`
must call `cache.Invalidate(inst.UUID)` immediately after `inst.Hibernate()` returns
successfully (whether from resource-pressure path or idle-timeout path).

For manual hibernation via the RPC (`HibernateSession`), the `SessionService` does not
have a reference to the sweeper's cache. Two options:
1. Make the cache a shared package-level value that the service can also write to.
2. Return `memory_rss_mb = 0` server-side whenever `inst.IsHibernated()` is true,
   bypassing the cache entirely.

**Decision**: Option 2 is simpler and correct. In `adapters.InstanceToProto()`, if
`inst.IsHibernated()`, set `MemoryRssMb = 0` and `EstimatedSavingsMb = 0` regardless
of cache. The cache is only consulted for Active sessions.

---

## Pitfall 6: macOS — /proc Doesn't Exist

The requirements (NFR-2) state memory measurement must "fail gracefully (returning 0) on
macOS."

**Risk**: Any direct `/proc/meminfo` read without a build tag will cause a file-not-found
error on macOS at runtime (not a compile error).

**Mitigation**: 
1. Use gopsutil for all reads — `mem.VirtualMemory()` works on macOS via `sysctl`.
2. If direct `/proc` reads are used for performance, wrap with `//go:build linux` build tag.
3. The sweeper must handle `GetSystemMemoryPct()` returning 0 on macOS as a sentinel:
   ```go
   if pct == 0 {
       // Measurement unavailable — skip resource-pressure hibernation
       return
   }
   ```

**Sentinel value choice**: Return 0.0 for "unavailable" (not 100%). Returning 100% would
incorrectly trigger hibernation on macOS developer machines. Return 0.0 and document it as
"below threshold always" — no pressure hibernation on macOS.

**gopsutil on macOS**: `mem.VirtualMemory()` uses `host_statistics64` via CGo on macOS.
The `Used` percentage may differ slightly from Linux. Acceptable for UX purposes.

---

## Pitfall 7: Test Strategy — No Real tmux or /proc

Sweeper tests must not require a real tmux server or real `/proc`. The existing sweeper
test files (`session/instance_test.go`, `session/state_machine_test.go`) use in-memory
mocks for the storage layer.

**Risk**: Tests that exec tmux or read `/proc` break in CI containers and slow the test suite.

**Mitigation**:

1. **`MemoryReader` interface** (already in architecture.md): The sweeper accepts a
   `memory.Reader` interface. Tests inject a `FakeMemoryReader`:
   ```go
   type FakeMemoryReader struct {
       SystemPct float64
       ProcRSS   map[int32]int64
   }
   func (f *FakeMemoryReader) SystemMemory(_ context.Context) (memory.SystemStats, error) {
       return memory.SystemStats{UsedPct: f.SystemPct}, nil
   }
   func (f *FakeMemoryReader) ProcessMemory(_ context.Context, pid int32) (memory.ProcessStats, error) {
       rss, ok := f.ProcRSS[pid]
       if !ok { return memory.ProcessStats{}, nil }
       return memory.ProcessStats{RSSMB: rss}, nil
   }
   ```

2. **`LiveInstancesProvider` interface** (already on sweeper): Tests can inject fake
   instances with known `LastMeaningfulOutput` values.

3. **Test scenarios to cover**:
   - System memory below threshold → no pressure hibernation triggered
   - System memory above threshold + one idle session → that session hibernated
   - System memory above threshold + no idle sessions (all recent output) → none hibernated
   - Two idle sessions → only the longest-idle one hibernated per tick
   - After hibernation → `memory_rss_mb` returns 0 for hibernated instance

4. **tmux PID resolution in tests**: The tmux `list-panes` exec call must also be
   injectable. Pass a `PIDResolver` function/interface to the memory reader:
   ```go
   type PIDResolver func(ctx context.Context, sessionName string) ([]int32, error)
   ```
   Default implementation calls `tmux list-panes`. Tests return a fixed PID list.

---

## Pitfall 8: Sweeper Hibernating While HibernateSession RPC Is In-Flight

The sweeper uses `liveProvider.GetInstances()` (in-memory), while the RPC uses
`storage.LoadInstances()` (disk). If both try to hibernate the same session simultaneously:

- Sweeper calls `inst.Hibernate(ctx)` → transitions state to Hibernated.
- RPC concurrently calls `inst.Hibernate(ctx)` on a different `*Instance` from disk.

**Risk**: Double-kill of tmux session (second kill is a no-op, safe). Double checkpoint
write (second write overwrites first, safe). Double `SaveInstances` (last writer wins —
could revert the sweeper's save).

**Existing mitigation**: `transitionTo()` validates state transitions. `Hibernated →
Hibernated` is not a valid transition and returns `ErrInvalidTransition`. The second
caller gets an error, which the RPC returns as `CodeFailedPrecondition`. This is safe.

**No additional mitigation needed** — the state machine already handles this.

---

## Summary: Top 3 Risks

1. **tmux pane_pid = shell, not AI process** — must use `Children()` traversal to get
   accurate RSS. Failure to do this makes all memory readings useless (~4 MB per session).
2. **Cache not invalidated on hibernation** — mitigated by always returning 0 from
   `InstanceToProto` when `IsHibernated()`, bypassing the cache at the proto layer.
3. **macOS sentinel value** — returning 0.0 from `GetSystemMemoryPct()` on macOS must
   disable resource-pressure hibernation, not accidentally trigger it (never return 100.0).
