# Findings: Pitfalls - Known Failure Modes and Races

**Date**: 2026-04-15  
**Phase**: Pre-implementation research (Phase 4: Validation)  
**Status**: Documented from code review of plan.md + source implementation

---

## Executive Summary

The session resumption feature has **four documented known issues (KI-001 through KI-007)** with well-understood mitigations. Beyond these, **four new race conditions** emerge from code inspection of the linker, scrollback, and shutdown capture implementations. The highest-severity risks are **KI-003 (PID reuse)** and the **new HistoryLinker startup scan race (NEW-001)**, both capable of silent data corruption if not mitigated. Medium-severity risks center on **CGo build failures (KI-001)** and **concurrent reads during checkpoint creation (NEW-002)**. Low-severity risks are primarily related to file-watching flakiness and tmux lifecycle races.

**Recommendation**: All seven risks below should be addressed before Phase 1 MVP ships. KI-003, NEW-001, and NEW-002 are blockers; the others can be handled with defensive coding + testing infrastructure.

---

## Risk Catalog

### KI-001: gopsutil CGo Requirement on macOS/Windows [SEVERITY: Medium]

**Description**  
`gopsutil/v3` requires CGo on macOS and Windows to access process information via `proc_pidinfo` and Windows APIs. However, the current CI build workflow (`build.yml`) sets `CGO_ENABLED: 0` globally, which will fail any CGo-dependent code at link time.

**Trigger**
- PR adds `go get github.com/shirou/gopsutil/v3/process`
- CI runs `go build` with `CGO_ENABLED: 0` (line 116 in `.github/workflows/build.yml`)
- Link fails with undefined reference on cross-compile matrix (darwin/amd64, darwin/arm64)
- Linux/amd64 and Linux/arm64 may succeed if gopsutil provides pure-Go fallback, but macOS/Windows binaries will not ship

**Mitigation (Current Plan Status: PARTIAL)**

The plan mentions build tags (`//go:build darwin`) but does not specify exact implementation. Proposed mitigations:

1. **Option A: Build tag gating (recommended)**
   - Add `//go:build darwin || windows` to `session/procinfo/inspector.go`
   - Provide stub implementation for Linux that panics with clear error message
   - Enable `CGO_ENABLED=1` **only** in the macOS and Windows build matrix entries
   - Linux builds remain `CGO_ENABLED=0`

2. **Option B: Conditional dependency**
   - Use `go:build` to conditionally import gopsutil only on macOS/Windows
   - Implement `ProcessInspector` interface with two implementations:
     - `inspector_darwin.go` with real gopsutil calls
     - `inspector_linux.go` with stubs or `/proc` parsing fallback
   - Same approach for Windows

3. **Option C: Pure-Go fallback** (not recommended)
   - Attempt pure-Go process inspection via `/proc/<pid>/fd` on Linux
   - gopsutil will still need CGo on macOS
   - Extra implementation burden, unclear benefit

**Files Affected**
- `session/procinfo/inspector.go` (needs build tag)
- `.github/workflows/build.yml` (matrix needs conditional `CGO_ENABLED`)
- `go.mod`, `go.sum` (gopsutil dependency)

**Prevention**
- Update build workflow to detect CGo dependencies and conditionally enable CGo per platform
- Add `go build ./...` test step **without** `CGO_ENABLED=0` to catch build failures early
- Document in `CLAUDE.md`: "ProcessInspector only available on macOS/Windows; Linux uses fallback"

**Status**: Not yet implemented; blocker if ignored.

---

### KI-003: PID Reuse During History Correlation [SEVERITY: High]

**Description**  
Between the moment `HistoryLinker` detects a PID's open files and the moment it writes `SessionID` to the session record, the PID could be reused by a completely different process. This would silently link a Claude session to the wrong conversation history, or worse, correlate a session with a different program's file entirely.

**Trigger**
1. Claude session starts with PID 12345, creates JSONL file at `~/.claude/projects/myproject/abc123.jsonl`
2. HistoryLinker polls and calls `detector.Detect(12345)`, which returns `HistoryFileInfo{ConversationUUID: "abc123", ...}`
3. Before correlateSession writes this UUID, Claude process exits
4. Another process (e.g., build tool) reuses PID 12345
5. HistoryLinker sees PID 12345 has file handles open (belonging to the new process, not Claude)
6. Wrong conversation UUID is linked to the session

**Scenario Severity**: This is particularly dangerous because:
- It happens silently with no error message
- The wrong UUID persists in storage
- Cold restore will use the wrong conversation
- Undetectable until user notices conversation context is wrong

**Mitigation (Current Plan Status: DOCUMENTED, IMPLEMENTATION PARTIAL)**

The plan specifies using `Process.CreateTime()` as a timestamp check, documented in `session/procinfo/inspector.go`:

```go
// IsAlive(pid int32, expectedCreateTime int64) bool
// Checks PID reuse by verifying process creation time matches
```

**Code Review Findings**:
- `ProcessInspector.IsAlive()` is defined in the plan but its usage in `HistoryLinker.correlateSession()` is **not explicitly shown**
- `history_linker.go` lines 168-170 call `hl.detector.Detect(pid)` but the code does not verify create time before using the result
- **Gap**: No check that the PID's create time matches when the tmux pane was launched

**Corrected Mitigation**:
1. Store `{PID, CreateTimeMs}` tuple in `Instance` when tmux session starts
2. Before each `Detect()` call, verify `ProcessInspector.IsAlive(pid, expectedCreateTime)` returns true
3. If create time mismatch, skip correlation and log warning (PID was reused)
4. Add integration test with controlled PID reuse simulation (using mock gopsutil)

**Files Affected**
- `session/procinfo/inspector.go` - `IsAlive()` implementation must compare both PID and create time
- `session/instance.go` - store `{pid, createTimeMs}` tuple at tmux start time
- `session/history_linker.go` - call `IsAlive()` check before using `Detect()` result

**Prevention**
- Add a `ValidatePIDAlive()` helper that returns error if PID is stale
- Every `Detect()` call must be wrapped: `if !isPIDAlive(old pid, old create time) { skip correlation }`
- Unit test: mock `gopsutil.Process` with create time mismatch, verify correlation is skipped
- Integration test: launch Claude, grab PID/create time, kill process, launch new process with same PID, verify linker does not correlate

**Status**: Documented in plan; implementation details require verification. **BLOCKER if not verified before Phase 1.**

---

### KI-004: Partial JSONL Line During Concurrent Read [SEVERITY: Medium]

**Description**  
When `HistoryFileDetector` or `scrollback/fork.go` reads a Claude JSONL history file while Claude is actively writing to it, the last line of the file may be incomplete (partial JSON object). Attempting to unmarshal this incomplete line will produce a JSON decode error, potentially causing the history detection or checkpoint fork to fail.

**Trigger**
1. Claude writes message to `~/.claude/projects/myproject/abc123.jsonl`: `{"role": "user", "content": "hello"}`
2. Simultaneously, stapler-squad reads the file to detect history
3. Reader sees: `{"role": "user", "content": "hell` (line incomplete)
4. `json.Unmarshal()` fails with "unexpected EOF" or "invalid JSON"
5. If error is not handled, history detection fails and session is not linked

**Mitigation (Current Plan Status: DOCUMENTED, IMPLEMENTATION PRESENT)**

The plan specifies: "Use `bufio.Scanner` for line-by-line reading. Skip the last line if `json.Unmarshal()` fails."

**Code Review Findings**:
- `session/history_detector.go` is not yet written, so implementation cannot be verified
- The plan correctly identifies the pattern: read with Scanner, skip unparseable last line
- Existing code in `session/history.go` (ClaudeSessionHistory) may already have this pattern

**Corrected Mitigation**:
1. Implement utility `jsonl.ReadValidLines(reader io.Reader) ([][]byte, error)`
   - Uses `bufio.Scanner` with line-delimited JSON
   - Skips any line that fails `json.Unmarshal()`
   - Returns only fully-parseable lines
2. Both `HistoryFileDetector.Detect()` and `scrollback/fork.go` must use this utility
3. Add test with malformed last line: ensure no error, correct lines parsed

**Files Affected**
- `session/history_detector.go` - uses `ReadValidLines()` when scanning open JSONL files
- `session/scrollback/fork.go` (Phase 2) - uses `ReadValidLines()` when copying scrollback
- New utility: `session/jsonl/reader.go` (optional, or inline in detector)

**Prevention**
- Every JSONL reader must skip unparseable lines, not fail
- Add assertion: "No JSONL reader shall call raw `json.Unmarshal()` directly on reader input"
- Test case: hand-craft JSONL file with incomplete last line, verify read succeeds and returns all complete lines

**Status**: Documented; implementation required. Low risk because pattern is straightforward.

---

### KI-007: Cold Restore with Missing Working Directory [SEVERITY: Medium]

**Description**  
When cold-restoring a session after `WorkingDir` was captured from a tmux pane, the saved directory may no longer exist on disk. Common triggers:
- User manually deleted the directory
- A previous pause operation cleaned up the worktree directory
- CI ran in a temporary workspace that was torn down
- The user switched machines and the path is stale

If `WorkingDir` is passed directly to tmux without validation, Claude starts in a non-existent directory or tmux fails silently.

**Trigger**
1. HistoryLinker captures WorkingDir: `/Users/tyler/my-project/src`
2. User runs `rm -rf /Users/tyler/my-project`
3. stapler-squad restarts; cold restore tries to launch Claude in non-existent dir
4. tmux may start Claude in fallback directory (unpredictable)
5. User resumes in wrong context

**Mitigation (Current Plan Status: DOCUMENTED, PREVENTION IN PLACE)**

The plan specifies: `resolveStartPath()` already validates directory existence and falls back to `basePath` if `WorkingDir` does not exist.

**Code Review Findings**:
- The plan correctly identifies the mitigation: use `resolveStartPath()` which validates before passing to tmux
- No changes required beyond ensuring this is called in the cold restore path (`story 1.2.2a`)

**Corrected Mitigation**:
1. In `instance.start()` when `firstTimeSetup == false` and UUID is available:
   - Call `resolveStartPath()` (already exists) to validate WorkingDir
   - Log warning if WorkingDir did not exist and fallback was used
   - Pass validated path to tmux session launch
2. Add test case: WorkingDir points to missing dir, verify fallback to Path succeeds

**Files Affected**
- `session/instance.go` - ensure `resolveStartPath()` is called in cold restore branch

**Prevention**
- Document: "CaptureCurrentState captures WorkingDir but resolveStartPath validates at startup"
- Unit test: set WorkingDir to non-existent path, call Start(false), verify fallback logged

**Status**: Documented and likely already handled; verification needed.

---

### NEW-001: HistoryLinker Startup Scan Race [SEVERITY: Medium]

**Description**  
`HistoryLinker.Start()` performs an initial synchronous scan of all sessions before entering the poll loop. However, if `HistoryLinker.SetInstances()` or session loading from storage is not completed before `Start()` is called, the linker will scan a partial or empty session list. This race causes newly-loaded sessions to not be correlated with their JSONL files until the next poll interval (5 seconds).

**Trigger**
1. Server startup loads sessions from storage into session manager
2. Concurrently, `HistoryLinker.Start(ctx)` is called
3. Line 111: `hl.scanAllSessions()` acquires read lock on `hl.instances`
4. At that moment, `hl.instances` is still empty or partially filled (session loading not complete)
5. The synchronous scan completes with no sessions to correlate
6. Poll loop starts and will pick up sessions on next interval (5s delay)
7. User sees session with no linked conversation UUID for 5 seconds

**Scenario Severity**: Low visibility issue, but can cascade:
- User creates new session, waits for history link to appear
- Gets impatient, tries to cold restore before UUID is populated
- Cold restore fails because UUID is still empty

**Root Cause**
- `HistoryLinker.Start()` runs synchronously scan at startup
- Session loading from storage is asynchronous or not synchronized with linker startup
- No guarantee that `SetInstances()` has been called before `Start()` completes its initial scan

**Mitigation**
1. **Blocking initialization** (recommended): Pass already-loaded session list to `NewHistoryLinkerFromRealInspector()` or add a `SetInstances()` call **before** `Start()`
   ```go
   linker := NewHistoryLinkerFromRealInspector()
   linker.SetInstances(sessionManager.AllInstances())  // Block until set
   linker.Start(ctx)  // Now safe to scan
   ```
2. **Deferred scan**: Move initial scan into a separate method, call it after all sessions are loaded
   ```go
   linker.Start(ctx)  // Start poll loop only
   // ... later, after sessions loaded ...
   linker.ScanAll()   // Explicit initial scan
   ```
3. **Async startup coordination**: Add a `Ready()` channel that blocks until first scan is complete
   ```go
   ready := linker.Start(ctx)  // Returns channel
   <-ready  // Wait for first scan before serving requests
   ```

**Files Affected**
- `session/history_linker.go` - `Start()` method and session list synchronization
- `server/dependencies.go` or equivalent - startup sequence that wires linker

**Prevention**
- Document startup sequence: "HistoryLinker.SetInstances() must be called before Start()"
- Add assertion in `Start()`: `if len(hl.instances) == 0 { warn "HistoryLinker started with no instances" }`
- Integration test: verify that SetInstances is called before Start, and first scan includes all sessions

**Status**: Identified from code review. Requires implementation verification.

---

### NEW-002: Scrollback LatestSequence Race During Checkpoint Creation [SEVERITY: Low-Medium]

**Description**  
When `Instance.CreateCheckpoint()` captures the current scrollback sequence number, it reads `FileScrollbackStorage.LatestSequence()` while the live session is simultaneously appending new scrollback entries. If the read happens mid-append, the sequence number may be:
1. Off-by-one (between old and new sequence)
2. Inconsistent with the actual line count written to disk
3. Point to a partial entry that has not finished writing

This creates a checkpoint with an inconsistent scrollback snapshot. When fork (Phase 2) uses this sequence, it may read the wrong number of lines or corrupt the forked scrollback file.

**Trigger**
1. Live session appends message to scrollback: sequence 1000 → 1001
2. User clicks "Create Checkpoint"
3. `CreateCheckpoint()` calls `scrollbackStorage.LatestSequence(sessionID)`
4. Scrollback storage acquires lock, reads file, extracts sequence
5. Simultaneously, another append writes sequence 1001 to disk
6. LatestSequence returns 1000 (from stale read)
7. Checkpoint stored with `ScrollbackSeq: 1000`
8. Fork later tries to read lines [0..1000], misses new entries, or worse, reads partial line

**Scenario Severity**: Low probability during normal use, but high impact during fork:
- Forked session missing recent scrollback context
- Partial JSONL line corruption in forked file (if boundary is hit)

**Root Cause**
- `LatestSequence()` does not guarantee atomicity with live append
- No checkpoint-time lock preventing concurrent appends
- Sequence number tracking may lag behind actual file write

**Mitigation** (requires implementation verification)
1. **Snapshot at checkpoint time** (recommended):
   - `CreateCheckpoint()` must acquire scrollback write lock before reading sequence
   - Temporarily freeze appends, read current sequence, release lock
   - Document: "Checkpoint creation may block scrollback writes for <1ms"

2. **Atomic append + checkpoint**:
   - Add method `FileScrollbackStorage.AppendAndSnapshot(entries, checkpoint callback)` 
   - Callback runs **inside** the lock after entries are written
   - Ensures sequence is captured **after** append is durably written

3. **Sequence counter separate from file**:
   - Add in-memory `atomicUint64` counter for sequence
   - Increment before each append (under lock)
   - Read counter atomically in `LatestSequence()` (no file I/O needed)
   - Trade: extra RAM for sequence tracking, but simpler semantics

**Files Affected**
- `session/scrollback/storage.go` - `LatestSequence()` implementation and lock strategy
- `session/instance.go` - `CreateCheckpoint()` must coordinate with scrollback storage
- `session/checkpoint.go` - ensure sequence is atomic snapshot

**Prevention**
- Add `// SequenceNumber must be read under lock to avoid races` comment in storage
- Unit test: simulate concurrent append + checkpoint, verify sequence is consistent
- Integration test: heavy write load, concurrent checkpoints, verify no sequence corruption

**Status**: Identified from code review. Requires detailed implementation review of scrollback storage locks.

---

### NEW-003: CaptureCurrentState Race After Tmux Death [SEVERITY: Low]

**Description**  
`Instance.CaptureCurrentState()` is called during graceful shutdown to capture the tmux pane's current working directory. However, if the server context is cancelled or the shutdown sequence is interrupted, `CaptureCurrentState()` may attempt to query a tmux session that was killed between the `DoesSessionExist()` check and the actual `GetPaneCurrentPath()` call.

**Trigger**
1. Graceful shutdown begins
2. Shutdown handler calls `CaptureCurrentState()` for all instances
3. Thread A: `CaptureCurrentState()` checks `i.tmuxManager.DoesSessionExist()` → true
4. Concurrently, tmux server crashes or is killed by supervisor
5. Thread A: calls `tmuxSession.GetPaneCurrentPath()` → error (tmux dead)
6. Error handling logs warning, but WorkingDir remains stale
7. Cold restore uses wrong directory on next startup

**Scenario Severity**: Low probability and low impact:
- Only triggers if tmux is killed during shutdown window
- Graceful degradation to fallback path is acceptable
- User may restart in slightly wrong directory, but generally OK

**Root Cause**
- TOCTOU race: `DoesSessionExist()` check followed by separate `GetPaneCurrentPath()` call
- No atomicity guarantee between the two operations
- Tmux is a separate process that can die unexpectedly

**Mitigation**
1. **Atomic session query**: Combine existence check + path read into single tmux call
   - Single call to `tmux display-message -p -t <session> '#{pane_current_path}'`
   - If session missing, single error returned (no TOCTOU)

2. **Graceful degradation**: Already partially handled
   - `CaptureCurrentState()` returns nil error if tmux is dead
   - `WorkingDir` remains unchanged (fallback to Path on cold restore)
   - Add log: "Could not capture current state for '<session>' — tmux may have crashed"

3. **Timeout and retry**: On error, retry once with backoff (but don't delay shutdown)

**Files Affected**
- `session/instance.go` - `CaptureCurrentState()` method
- `session/tmux/tmux.go` (or equivalent) - ensure atomic session queries

**Prevention**
- Add test case: kill tmux between DoesSessionExist and GetPaneCurrentPath, verify no panic
- Document: "CaptureCurrentState gracefully degrades if tmux dies"

**Status**: Identified from code review. Likely already handled correctly (returns nil on error), but should verify.

---

### NEW-004: fsnotify Test Flakiness Under High CI Load [SEVERITY: Low]

**Description**  
Tests for `HistoryFileWatcher` that create temp files and expect fsnotify callbacks may be flaky in CI environments. macOS FSEvents coalesces rapid file changes and may batch multiple CREATE events into a single callback, or delay callbacks by 1-3 seconds during high system load.

**Trigger**
1. Test creates `.jsonl` file in temp dir
2. Expects fsnotify callback to fire within a short timeout (e.g., 100ms)
3. Under high CI load, FSEvents may delay event by 2-3 seconds
4. Test times out and fails intermittently
5. Retrying later succeeds (race condition)

**Scenario Severity**: Low — test infrastructure issue only, not production code
- Users do not write tests directly; framework handles it
- But intermittent CI failures block PRs and reduce confidence

**Root Cause**
- FSEvents (macOS) and inotify (Linux) have different behavioral guarantees
- High system load delays event delivery
- Tests written with hard-coded timeouts that assume immediate delivery

**Mitigation**
1. **Longer timeouts in tests**: Use 5-10 second timeout instead of 100ms
   - Acceptable for test execution time
   - Reduces false negatives

2. **Event counting with retry**: Instead of waiting for callback to fire immediately
   ```go
   // Instead of:
   select { case <-callback: ... }
   
   // Do:
   for i := 0; i < 10; i++ {
     if callbackFired { break }
     time.Sleep(500 * time.Millisecond)
   }
   ```

3. **Skip fsnotify tests in CI**: Mark with build tag `//go:build !ci`
   - Use conditional environment variable to skip in CI
   - Still run locally to catch basic issues

4. **Mock fsnotify in tests**: Replace real fsnotify with mock that fires immediately
   - More deterministic than relying on OS events
   - Recommended for unit tests

**Files Affected**
- `session/history_watcher_test.go` - timeout values and test structure
- CI workflow - consider conditional test skipping

**Prevention**
- Document: "fsnotify-based tests use 5s timeout to account for system load"
- Add `// +build !ci` to skip in CI if real FS tests are not critical
- Unit test: mock fsnotify, verify callback fires correctly
- Integration test (local-only): real fsnotify with generous timeout

**Status**: Identified from code review. Low priority but worth documenting.

---

## Trade-off Matrix

| Risk ID | Severity | Likelihood | Impact | Complexity | Mitigation Cost | Priority |
|---------|----------|------------|--------|-----------|-----------------|----------|
| KI-001 | Medium | High | CI build fails | Low | Low | P2 |
| KI-003 | High | Medium | Silent data corruption | Medium | Medium | P1 |
| KI-004 | Medium | Low | History detection fails | Low | Low | P2 |
| KI-007 | Medium | Low | Wrong start directory | Low | Very Low | P3 |
| NEW-001 | Medium | Low | 5s delay in linkage | Low | Low | P2 |
| NEW-002 | Low-Med | Very Low | Forked scrollback inconsistency | Medium | Medium | P3 |
| NEW-003 | Low | Very Low | Race during shutdown | Low | Low | P3 |
| NEW-004 | Low | Low | CI flakiness | Low | Low | P3 |

**P1 (Blockers)**: KI-003 (PID reuse detection)  
**P2 (Pre-release)**: KI-001 (CGo), KI-004 (JSONL parsing), NEW-001 (startup race)  
**P3 (Nice-to-have)**: KI-007, NEW-002, NEW-003, NEW-004

---

## Mitigation Priority and Implementation Order

### Phase 1a: Validation (Before Code)

1. **Verify KI-001 impact** (30 min)
   - Check if any existing code uses CGo
   - Determine if CI needs `CGO_ENABLED=1` globally
   - Decision: build tag gate vs. pure-Go fallback

2. **Verify KI-003 flow** (30 min)
   - Confirm PID + CreateTime tuple is stored at tmux start
   - Confirm `IsAlive()` is called before each Detect
   - Add assertion in tests: every PID must have create time validation

3. **Verify NEW-001 coordination** (30 min)
   - Confirm SetInstances is called before Start in server startup
   - Trace startup sequence in `server/dependencies.go` or `main.go`
   - Add synchronization point if missing

### Phase 1b: Implementation (Story Order)

Follow existing plan order with added safeguards:

1. **Story 1.1.1**: Add gopsutil with conditional build tags
   - Add `//go:build darwin || windows` to procinfo
   - Update CI to enable CGo only for darwin/windows matrix entries

2. **Story 1.1.2**: Implement HistoryFileDetector with PID validation
   - Add PID + CreateTime tuple checks before using Detect results
   - Return error if PID create time mismatch

3. **Story 1.1.3**: Implement HistoryFileWatcher with generous timeouts
   - Use 5s timeout in tests
   - Mark fsnotify tests with build tag for optional CI skipping

4. **Story 1.1.4**: Implement HistoryLinker with startup coordination
   - Add assertion: instances not empty at start
   - Add log statement if no instances found
   - Ensure SetInstances called before Start in main

5. **Story 1.2.1**: Implement CaptureCurrentState with graceful degradation
   - Handle tmux death gracefully (return nil, log warning)
   - Verify atomic session query if possible

6. **Story 1.3.2**: Implement CreateCheckpoint with scrollback atomicity
   - Ensure scrollback sequence is read under lock
   - Consider snapshot callback pattern

### Phase 1c: Testing

Add test cases for each risk:

```go
// KI-003 test
func TestHistoryLinkerPIDReuse(t *testing.T) {
  // Mock gopsutil with create time mismatch
  // Verify correlation is skipped
}

// NEW-001 test
func TestHistoryLinkerStartupNoInstances(t *testing.T) {
  linker := NewHistoryLinker(...)
  linker.Start(ctx)  // instances empty
  // Verify log warning or assertion
}

// KI-004 test
func TestHistoryDetectorPartialJSONL(t *testing.T) {
  // Hand-craft file with incomplete last line
  // Verify detector skips partial line, returns valid entries
}

// NEW-002 test
func TestCheckpointWithConcurrentScrollback(t *testing.T) {
  // Heavy scrollback writes + concurrent checkpoint creation
  // Verify sequence is consistent
}
```

---

## Open Questions

- [ ] **KI-001**: Is gopsutil CGo already handled by conditional build tags in `procinfo/inspector.go`? Or is this a TODO?
- [ ] **KI-003**: Is PID + CreateTime tuple stored at tmux start? Where? (Need to verify in instance.go)
- [ ] **NEW-001**: Is SetInstances called before Start in server startup? Trace sequence in dependencies.go
- [ ] **NEW-002**: How is scrollback sequence atomicity currently handled? Is there a test for concurrent appends?
- [ ] **NEW-003**: Does CaptureCurrentState currently handle tmux death gracefully? (Appears to based on code, but verify)

---

## Risk Mitigation Checklist (Pre-Phase 1 Implementation)

- [ ] KI-001: CGo build tag strategy decided and CI updated
- [ ] KI-003: PID + CreateTime validation implemented and tested
- [ ] KI-004: JSONL partial line handling implemented and tested
- [ ] KI-007: resolveStartPath called in cold restore branch, verified with test
- [ ] NEW-001: Startup synchronization verified, SetInstances before Start
- [ ] NEW-002: Scrollback sequence atomicity verified, test added for concurrent appends
- [ ] NEW-003: CaptureCurrentState error handling verified, tmux death handled gracefully
- [ ] NEW-004: fsnotify tests use generous timeout or mocked fsnotify

---

## Recommended Implementation Sequence

1. **First sprint**: Fix KI-001 (CGo), verify KI-003 (PID validation)
2. **Second sprint**: Implement stories 1.1.1 through 1.1.4 with safeguards
3. **Third sprint**: Implement stories 1.2.1 through 1.3.4 with additional race condition testing
4. **Before release**: Run full test suite with `-race` flag, manual testing under high load

---

## Prior Art and Lessons Learned

**gopsutil CGo Issues** [TRAINING_ONLY - verify]
- Known limitation on macOS/Windows; Linux has pure-Go fallback in some versions
- Projects using gopsutil typically build with platform-specific CGo settings
- Reference: shirou/gopsutil#1234 (conditional compilation for cross-platform builds)

**PID Reuse Races** [TRAINING_ONLY - verify]
- Common pitfall in process monitoring tools
- Go stdlib's `os.Process` does not prevent PID reuse races
- Mitigation: always verify process creation time alongside PID

**fsnotify Flakiness** [TRAINING_ONLY - verify]
- Known issue on macOS FSEvents; inotify on Linux is more reliable
- High load causes event coalescing and delays
- Projects like Docker, Kubernetes use custom event deduplication + timeouts

---

## Pending Web Searches

1. `gopsutil CGo github actions cross-compile darwin windows 2025`  
   → Verify CI build strategy for conditional CGo enablement

2. `go process PID reuse race condition mitigation CreateTime`  
   → Verify best practices for PID validation

3. `fsnotify macos FSEvents coalescing test flakiness`  
   → Understand event delivery guarantees under load

4. `concurrent file read write json JSONL partial line handling go`  
   → Confirm bufio.Scanner is recommended pattern

---

## Conclusion

The session resumption feature has well-documented mitigations for known issues, but **three critical gaps require verification before implementation**:

1. **KI-003 (PID reuse)**: Must verify CreateTime validation is actually implemented
2. **NEW-001 (startup race)**: Must verify SetInstances is called before Start
3. **NEW-002 (scrollback atomicity)**: Must verify scrollback lock strategy during checkpoint creation

All other risks are manageable with defensive coding and comprehensive testing. Recommend a 2-week validation sprint before Phase 1 MVP implementation begins.

