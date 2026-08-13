# tmux Subprocess Optimization

## Epic Overview

### Problem

Execution trace profiling shows ~138 subprocess spawns/second at idle, traced to
`os/exec.(*Cmd).Start.func2` (277 goroutines). `ReviewQueuePoller.pollLoop` consumes
57% of all CPU execution time (5.3s/10s in syscalls). Two callsites drive the majority:

1. `inst.Preview()` in `session/instance.go` — calls `tmux capture-pane` for every
   session without an active `ClaudeController` on every 2-second poll tick.
2. `worktree.IsDirty()` in `session/git/worktree_git.go` — calls `git status --porcelain`
   for every session with a git worktree, including sessions where Claude is actively
   generating output (and cannot change worktree state).

A third contributor: `CheckGHAuth()` in `github/client.go` runs `gh auth status` and
holds a shared mutex, causing 2.02s cumulative mutex delay per 10s profiling window.

The root cause for tmux calls: `controlModeStdin` is open and connected to the already-running
`tmux -C attach-session` process, but has never been used to send commands. The control mode
protocol accepts commands over stdin and returns `%begin`/`%end`-delimited responses on
stdout — a zero-subprocess path sitting unused.

### Success Metrics

| Metric | Baseline | Target |
|--------|----------|--------|
| Subprocess spawns at idle | ~138/s | <10/s |
| `ReviewQueuePoller` CPU | 57% (5.3s/10s) | <15% |
| `CheckGHAuth` mutex delay | 2.02s/10s window | <50ms/10s window |
| `IsDirty()` subprocess calls | every 2s per session | ≤1 per 15s per session |
| `Preview()` subprocess calls | every 2s per session | ≤1 per 500ms per session |

### Architecture Decision Records

The approach and trade-offs are documented in two ADRs. Do not recreate them here.

- `project_plans/tmux-optimization/decisions/ADR-001-control-mode-command-dispatch.md` —
  Phase 2: wire `controlModeStdin` for command/response dispatch, eliminating all tmux
  subprocess forks for query operations.
- `project_plans/tmux-optimization/decisions/ADR-002-ttl-caching-subprocess-reduction.md` —
  Phase 1: TTL caching for `IsDirty()`, `Preview()`, and `CheckGHAuth()`, shipping
  independently of ADR-001 with zero protocol risk.

### Delivery Phases

```
Phase 1: TTL Caching (1-2 days)
  1a. IsDirty() TTL cache           ─┐ parallel
  1b. CheckGHAuth() singleflight    ─┤ parallel
  1c. Preview() result cache        ─┘ parallel

         │ Integration checkpoint: measure subprocess rate after Phase 1
         │

Phase 2: Control Mode Command Dispatch (3-5 days)
  2a. Infrastructure + GetPaneDimensions (pilot)
         │ Feature-flag validation window (24h parallel paths)
  2b. display-message call sites (4 functions)
  2c. capture-pane call sites (3 functions)
  2d. Fire-and-forget call sites (2 functions)

         │ Integration checkpoint: verify CM correctness before full rollout

Phase 3: Adaptive Poll Interval (1 day, after Phase 1)
  3a. EventBus wiring into ReviewQueuePoller
```

---

## Phase 1: TTL Caching

### Story 1a — `IsDirty()` TTL Cache

**Goal**: Eliminate ~50% of subprocess forks by caching the `git status --porcelain`
result and skipping it entirely when Claude is actively generating output.

**Context files to load**:
- `session/git/worktree.go` — `GitWorktree` struct definition
- `session/git/worktree_git.go` — `IsDirty()` implementation (line 113)
- `session/instance.go` — how `IsDirty()` is called (search for `IsDirty`)
- `project_plans/tmux-optimization/decisions/ADR-002-ttl-caching-subprocess-reduction.md`

**Tasks**:

1. **Add cache fields to `GitWorktree` struct** (`session/git/worktree.go`)
   - Add `isDirtyCache bool`, `isDirtyCacheTime time.Time`, `isDirtyCacheTTL time.Duration`
   - Add `isDirtyCacheMu sync.RWMutex` — existing `runGitCommand` does not hold a lock,
     so the cache state needs its own protection
   - Initialize `isDirtyCacheTTL = 15 * time.Second` in constructors
   - Estimated: 1-2 hours

2. **Rewrite `IsDirty()` with TTL + active-Claude skip** (`session/git/worktree_git.go`)
   - Fast path: read lock, return cached value if within TTL
   - Active-Claude skip: accept an `isClaudeActive bool` parameter or a `func() bool`
     callback so the caller can signal "skip, Claude is running". Prefer the callback
     to avoid coupling the git package to the session package.
   - On cache miss: acquire write lock, double-check, run subprocess, update cache
   - Estimated: 2-3 hours

3. **Wire the active-Claude signal** (`session/instance.go`)
   - Where `IsDirty()` is called in the poll path, pass whether `ClaudeController`
     reports active (use `inst.claudeController != nil && inst.claudeController.IsActive()`)
   - Estimated: 30 minutes

4. **Add cache hit/miss logging** (`session/git/worktree_git.go`)
   - Log at DEBUG level on cache miss, including time since last subprocess call
   - This is required for the Phase 1 integration checkpoint
   - Estimated: 30 minutes

**Verification**: After merging, run the application for 10 minutes with debug logging
enabled and confirm `IsDirty()` cache miss rate drops to approximately 1/15s per session
rather than 1/2s.

---

### Story 1b — `CheckGHAuth()` Singleflight + Atomic TTL

**Goal**: Eliminate the 2.02s cumulative mutex delay by replacing the implicit per-call
`exec.Command` serialization with a non-blocking atomic read + singleflight for expiry.

**Context files to load**:
- `github/client.go` — `CheckGHAuth()` implementation (line 113)
- `project_plans/tmux-optimization/decisions/ADR-002-ttl-caching-subprocess-reduction.md`
- ADR-002 "CheckGHAuth singleflight + atomic TTL" section

**Tasks**:

1. **Add package-level atomic state and singleflight group** (`github/client.go`)
   - Define `type ghAuthState struct { ok bool; expiry time.Time }`
   - `var ghAuthCache atomic.Value` — stores `*ghAuthState`, nil means never checked
   - `var ghAuthGroup singleflight.Group`
   - Add `import "golang.org/x/sync/singleflight"` (already a transitive dep; verify
     with `go list -m golang.org/x/sync`)
   - Estimated: 1 hour

2. **Rewrite `CheckGHAuth()` with fast path**
   - Fast path: load atomic, if non-nil and not expired return cached `ok` value
   - On expiry or first call: `ghAuthGroup.Do("auth", func() { ... run subprocess, store atomic })`
   - TTL: 5 minutes (auth tokens do not change frequently)
   - The function signature stays `func CheckGHAuth() error` — callers unchanged
   - Estimated: 1-2 hours

3. **Add cache invalidation function** (`github/client.go`)
   - `func InvalidateGHAuthCache()` — stores nil into the atomic
   - Called by any codepath that explicitly logs out or changes auth state
   - Estimated: 30 minutes

**Verification**: Run the profile capture (`make restart-web-profile`) with 5+ sessions
active. The `CheckGHAuth` mutex delay in the goroutine dump should be absent or
negligible (<50ms) after this change.

---

### Story 1c — `Preview()` Result Cache

**Goal**: Skip `tmux capture-pane` subprocess calls for sessions whose terminal
content has not changed since the last poll tick.

**Context files to load**:
- `session/instance.go` — `Preview()` implementation (line 1242)
- `session/review_queue_poller.go` — how `Preview()` is called; note the existing
  `cachedContent` and `lastSeenActivity` maps (lines 64-68) — **do not duplicate
  this cache; extend it instead**
- `project_plans/tmux-optimization/decisions/ADR-002-ttl-caching-subprocess-reduction.md`

**Tasks**:

1. **Audit the existing content cache in `ReviewQueuePoller`** (`session/review_queue_poller.go`)
   - Lines 64-68 already define `cachedContent map[string]string` and
     `lastSeenActivity map[string]time.Time`. Read the full `pollLoop` to understand
     when this cache is consulted vs. bypassed.
   - Identify which branch of `pollLoop` calls `Preview()` without going through the
     cache (non-controller sessions where `lastSeenActivity` is not updated).
   - Estimated: 1 hour (read + analysis)

2. **Extend the existing cache to cover non-controller sessions**
   - For sessions without a `ClaudeController`, store a `previewCacheTime time.Time`
     per session and skip the `Preview()` subprocess call when `time.Since(previewCacheTime) < 500ms`
   - If the existing `lastSeenActivity` map is already keyed per session and updated
     from PTY events, prefer reusing it with a staleness check rather than adding a
     new map
   - Estimated: 2-3 hours

3. **Ensure cache is invalidated on session restart/resume** (`session/review_queue_poller.go`)
   - Call the appropriate invalidation path in `RemoveInstance` or when a session
     transitions from Paused to Running
   - Estimated: 30 minutes

**Verification**: With debug logging, confirm `Preview()` is only called when the
cached content timestamp exceeds 500ms.

---

### Phase 1 Integration Checkpoint

Before starting Phase 2, verify Phase 1 is working:

1. Run with profiling: `make restart-web-profile`
2. Let idle for 2 minutes with 5 active sessions
3. Capture goroutine dump: `curl http://localhost:6060/debug/pprof/goroutine?debug=2 > goroutines.txt`
4. Confirm: goroutine count for `os/exec.(*Cmd).Start.func2` is <50 (was 277)
5. Check debug logs for `IsDirty()` cache hit rate
6. Confirm: no `CheckGHAuth` mutex contention in goroutine dump

If subprocess count is not below 50, investigate before proceeding to Phase 2.

---

## Phase 2: Control Mode Command Dispatch

### Story 2a — CM Dispatch Infrastructure + Pilot (`GetPaneDimensions`)

**Goal**: Wire the FIFO request/response multiplexer into `processControlModeLine()`,
validate the protocol against a single low-risk call site, and establish the feature
flag pattern for the remaining 8 migrations.

**Context files to load**:
- `session/tmux/control_mode.go` — full file; `processControlModeLine()` is the
  extension point (line 222); `controlModeStdin` is already stored on `TmuxSession`
- `session/tmux/tmux.go` — `TmuxSession` struct (line 36); `GetPaneDimensions()` (line 1633)
- `project_plans/tmux-optimization/decisions/ADR-001-control-mode-command-dispatch.md`
- `project_plans/tmux-optimization/research/findings-control-mode-commands.md`

**Tasks**:

1. **Add dispatch infrastructure fields to `TmuxSession`** (`session/tmux/tmux.go`)
   ```go
   // Control mode command dispatch (ADR-001)
   pendingCmds []chan cmdResult  // FIFO queue; protected by controlModeSubMu
   cmdBodyBuf  bytes.Buffer     // accumulates body lines between %begin/%end
   inCmdResp   bool             // state machine: are we inside a %begin..%end block?
   ```
   Where `cmdResult` is a new unexported type:
   ```go
   type cmdResult struct {
       body string
       err  error  // populated by %error responses
   }
   ```
   `pendingCmds` is a `[]chan cmdResult` (slice as FIFO queue). tmux guarantees sequential
   ordering, so the head of the slice always corresponds to the in-flight command.
   - Estimated: 1 hour

2. **Implement `sendCMCommand()` helper** (`session/tmux/control_mode.go`)
   - Acquires `controlModeSubMu` write lock, appends a new `chan cmdResult` to `pendingCmds`,
     writes the command string + `"\n"` to `controlModeStdin`, releases lock
   - Blocks on the returned channel for a response (caller provides context for timeout)
   - Returns `("", ErrControlModeNotRunning)` when `controlModeStdin` is nil — this is
     the fallback signal for call sites to use the subprocess path
   - Estimated: 2-3 hours

3. **Extend `processControlModeLine()` with `%begin`/`%end` state machine**
   (`session/tmux/control_mode.go`, line 295 — currently `case "%begin", "%end": return`)
   - `%begin`: set `inCmdResp = true`, reset `cmdBodyBuf`
   - Body lines while `inCmdResp`: append to `cmdBodyBuf`
   - `%end`: set `inCmdResp = false`, pop head of `pendingCmds` (under write lock),
     send `cmdResult{body: cmdBodyBuf.String()}` to the channel, reset `cmdBodyBuf`
   - `%error` while `inCmdResp`: send `cmdResult{err: ...}`, reset state
   - Edge case: `%output` arriving while `inCmdResp` is true — `%output` notifications
     are for streaming pane output, not command responses. They must still be broadcast
     to subscribers even during a command response window. Do not suppress them.
   - Estimated: 3-4 hours

4. **Migrate `GetPaneDimensions()` behind feature flag** (`session/tmux/tmux.go`, line 1633)
   - Check `os.Getenv("STAPLER_SQUAD_CM_COMMANDS") == "true"` (or a package-level
     `atomic.Bool` set at startup for performance)
   - CM path: `sendCMCommand(ctx, "display-message -p -t SESSION #{pane_width} #{pane_height}")`
   - Parse response identically to the existing subprocess path
   - Subprocess fallback: existing `buildTmuxCommand(...)` path unchanged
   - For the 24-hour parallel validation window: run both paths, log any output
     discrepancy at WARN level, return the subprocess result (trusted baseline)
   - Estimated: 2-3 hours

5. **Add unit tests for the state machine** (`session/tmux/control_mode_test.go`)
   - Test: single command, response arrives correctly
   - Test: two commands queued, responses arrive in order
   - Test: `%error` response propagated to waiting channel
   - Test: `%output` during command response does not corrupt queue
   - Test: fallback when `controlModeStdin` is nil
   - Estimated: 2-3 hours

**Verification**: Enable `STAPLER_SQUAD_CM_COMMANDS=true`, start a session, call
`GetPaneDimensions()` via the web UI resize path. Confirm logs show CM path response
matches subprocess path response for 24 hours before proceeding to 2b.

**Open questions (must resolve before 2b)**:
- Does `display-message -p -t SESSION "#{pane_width} #{pane_height}"` parse correctly
  over CM stdin without shell quoting? (The format string contains spaces; test whether
  tmux treats the space as argument separator or as part of the format.)
- If shell quoting is required, the CM command must quote the format string with `'` or
  the `display-message` call must use multiple `-p` flags.

---

### Story 2b — Migrate `display-message` Call Sites (4 functions)

**Goal**: After the pilot validates the protocol, migrate the remaining four
`display-message`-based functions.

**Prerequisite**: Story 2a complete and CM paths validated for 24h. Open question
about `display-message` format string quoting must be resolved.

**Context files to load**:
- `session/tmux/tmux.go` — `GetCursorPosition()` (line 1612), `GetPaneCurrentPath()` (line 1762),
  `GetPanePID()` (line 1776), `SetWindowSize()` (line 1279) — read the full function bodies
  to understand the parse logic and existing error handling
- `session/tmux/control_mode.go` — `sendCMCommand()` from Story 2a

**Tasks**:

1. **Migrate `GetCursorPosition()`** — `display-message -p -t SESSION "#{cursor_x} #{cursor_y}"`
   - Same parsing as `GetPaneDimensions()` (two integers)
   - Estimated: 1 hour

2. **Migrate `GetPaneCurrentPath()`** — `display-message -p -t SESSION "#{pane_current_path}"`
   - Single string result; trim whitespace
   - Estimated: 1 hour

3. **Migrate `GetPanePID()`** — `display-message -p -t SESSION "#{pane_pid}"`
   - Single integer result; parse with `strconv.ParseInt`
   - Estimated: 1 hour

4. **Migrate `SetWindowSize()` (partial)** — `resize-window -t SESSION -x W -y H`
   - `SetWindowSize()` currently calls `GetPaneDimensions()` twice (before and after
     resize for verification). After 2a, those calls already use CM. Only the
     `resize-window` command itself needs migration here.
   - This is a fire-and-forget write command — no response body is expected. Send the
     command, then send a follow-up `display-message` to confirm the resize took effect
     rather than relying on the `%end` response (which for write commands is an empty body).
   - Estimated: 2 hours

**Verification**: Run 5 sessions through normal workload for 30 minutes. Confirm no
discrepancy warnings in logs for these four functions. Subprocess rate should be visibly
lower for sessions where the web UI polls dimensions on every resize event.

---

### Story 2c — Migrate `capture-pane` Call Sites (3 functions)

**Goal**: Migrate the highest-volume subprocess call sites — the three `capture-pane`
variants — to the CM dispatch path.

**Prerequisite**: Story 2b complete and stable. Open question about `capture-pane -S/-E`
flags over CM stdin must be resolved.

**Context files to load**:
- `session/tmux/tmux.go` — `CapturePaneContent()` (line 1540), `CapturePaneContentRaw()` (line 1560),
  `CapturePaneContentWithOptions()` (line 1577)
- `session/tmux/control_mode.go` — `sendCMCommand()` and `cmdBodyBuf` handling from 2a

**Tasks**:

1. **Verify multi-line response handling in the state machine**
   - `capture-pane` output can be hundreds of lines. The `cmdBodyBuf` accumulation
     in `processControlModeLine()` must handle lines that contain `%` as first character
     (legitimate terminal content). Confirm tmux escapes literal `%` in body content
     within `%begin`/`%end` blocks (review tmux source or wiki before implementing).
   - Estimated: 1 hour (research + verification)

2. **Migrate `CapturePaneContent()`** — `capture-pane -p -e -J -t SESSION`
   - CM command: `capture-pane -p -e -J -t SESSION`
   - Response body is the pane content as a multi-line string
   - Apply existing `sanitizeUTF8String()` to the response body
   - Estimated: 2 hours

3. **Migrate `CapturePaneContentRaw()`** — `capture-pane -p -e -t SESSION` (no `-J`)
   - Same pattern as above without the join-lines flag
   - Estimated: 1 hour

4. **Migrate `CapturePaneContentWithOptions()`** — `capture-pane -p -e -J -S start -E end -t SESSION`
   - Includes `-S` and `-E` range flags. Confirm these work over CM stdin before merging
     (this is the open verification question from ADR-001).
   - Estimated: 1-2 hours

**Verification**: This is the highest-impact migration. After enabling, capture a goroutine
dump at idle and confirm `os/exec.(*Cmd).Start.func2` goroutines drop to <10. Compare
`capture-pane` output via CM path against subprocess path for a TUI-heavy session (e.g.,
a running Claude session mid-generation) to verify ANSI sequences are preserved correctly.

---

### Story 2d — Migrate Fire-and-Forget Call Sites (2 functions)

**Goal**: Migrate `RefreshClient()` and eliminate the last two subprocess call sites.

**Prerequisite**: Stories 2b and 2c complete.

**Context files to load**:
- `session/tmux/tmux.go` — `RefreshClient()` (line 1516); note the fallback path that
  sends SIGWINCH — this fallback remains useful and should be kept
- `project_plans/tmux-optimization/decisions/ADR-001-control-mode-command-dispatch.md`
  (consequences section regarding `refresh-client` from the CM connection itself)

**Tasks**:

1. **Resolve the `refresh-client` self-reference question**
   - `refresh-client -t SESSION` sent from a CM connection that is itself attached to
     that session. Verify tmux behavior: does this work, is it a no-op, or does it
     cause an error? Test interactively before implementing.
   - If it causes an error, the correct CM command may be `refresh-client` without
     `-t` (which refreshes the current client), or the subprocess fallback should
     be retained permanently for this function.
   - Estimated: 1 hour (testing)

2. **Migrate `RefreshClient()`** — `refresh-client -t SESSION`
   - Fire-and-forget: send command, do not wait for response body (but do wait for
     the empty `%end` to confirm tmux received it)
   - Keep the SIGWINCH fallback path for when CM is not running
   - Estimated: 1-2 hours

**Verification**: Resize the browser window and confirm the terminal redraws correctly.
Confirm no subprocess spawns for refresh during a resize cycle.

---

### Phase 2 Integration Checkpoint

Before removing the feature flag and enabling CM dispatch by default:

1. Run with `STAPLER_SQUAD_CM_COMMANDS=true` for 48 hours across multiple sessions
2. Check for discrepancy warnings in logs (parallel path comparison from Story 2a)
3. Capture goroutine dump and confirm subprocess rate is <10/s at idle
4. Confirm no panics or deadlocks in the CM dispatch state machine under concurrent load
5. Remove parallel-path logging, set `STAPLER_SQUAD_CM_COMMANDS=true` as default,
   keep the `=false` escape hatch for rollback

---

## Phase 3: Adaptive Poll Interval

### Story 3a — EventBus-Driven Adaptive Interval in `ReviewQueuePoller`

**Goal**: Reduce baseline subprocess activity by 4x for the idle case (no sessions
awaiting approval) by backing the poll interval off from 2s to 8s, snapping back
to 2s only when events signal user-relevant activity.

**Prerequisite**: Phase 1 complete (reduces subprocess cost per tick; Phase 3 reduces
tick frequency). Phase 2 is not required — this ships independently.

**Context files to load**:
- `session/review_queue_poller.go` — full file; `ReviewQueuePollerConfig` (line 14),
  `pollLoop` (line 209), `Start()` (line 144)
- Search for `EventBus`, `EventApprovalResponse`, `EventUserInteraction` in the
  codebase to find the existing event infrastructure:
  `grep -rn "EventBus\|EventApproval\|EventUserInteraction" session/`

**Tasks**:

1. **Audit the EventBus API** (30 minutes)
   - Determine how to subscribe to `EventApprovalResponse` and `EventUserInteraction`
   - Confirm whether `ReviewQueuePoller` already has access to the bus, or needs it
     injected at construction time

2. **Add adaptive interval fields to `ReviewQueuePoller`**
   - `currentInterval time.Duration` — tracks active interval (starts at `config.PollInterval`)
   - `snapCh chan struct{}` — receives snap-to-fast signals from event handlers
   - `idleInterval time.Duration = 8 * time.Second` — configurable; add to `ReviewQueuePollerConfig`
   - Estimated: 1 hour

3. **Rewrite `pollLoop` to use adaptive timer**
   - Replace `time.Tick(config.PollInterval)` with a resettable timer
   - After each tick: check whether any session is in `NeedsApproval` state
     - Yes: ensure interval is `config.PollInterval` (2s)
     - No: back off to `idleInterval` (8s)
   - On `snapCh` receive: reset timer to `config.PollInterval` immediately
   - Estimated: 2-3 hours

4. **Wire EventBus subscriptions**
   - Subscribe to `EventApprovalResponse` and `EventUserInteraction` in `Start()`
   - On each event: non-blocking send to `snapCh`
   - Unsubscribe in `Stop()`
   - Estimated: 1-2 hours

5. **Update tests** (`session/review_queue_poller_test.go`)
   - Ensure existing poller tests still pass with adaptive interval
   - Add test: confirm interval backs off to 8s when no sessions are active
   - Add test: confirm snap to 2s on `EventApprovalResponse`
   - Estimated: 2 hours

**Verification**: Run with 5 idle sessions (no pending approvals). The poll tick logs
should show 8-second intervals. Approve a pending request and confirm the next tick
fires within 2s.

---

## Known Issues and Potential Bugs

### Concurrency: `%output` During `%begin`/`%end` Window

**Severity**: High

**Description**: The `processControlModeLine()` state machine will receive `%output`
notifications from pane activity while a command response is in-flight. The `%output`
handler calls `broadcastControlModeUpdate()` which acquires `controlModeSubMu` RLock.
The `%begin`/`%end` handler pops `pendingCmds` under `controlModeSubMu` WLock.
Both handlers run on the same goroutine (`readControlModeOutput`), so there is no
actual concurrent access — but any code that calls `sendCMCommand()` from an external
goroutine and then immediately tries to read subscriber output could race.

**Mitigation**: The single-goroutine processing of `processControlModeLine()` means
no interleaving within the parser. Verify that `sendCMCommand()` never acquires
`controlModeSubMu` WLock while the reader goroutine is inside `broadcastControlModeUpdate()`
(RLock). Since WLock and RLock on the same `sync.RWMutex` are mutually exclusive,
this is safe as long as `sendCMCommand()` does not hold WLock while waiting for the
response channel — ensure the lock is released before blocking on the channel.

**Files**: `session/tmux/control_mode.go` — `sendCMCommand()`, `processControlModeLine()`

### Concurrency: `StopControlMode()` While Command In-Flight

**Severity**: High

**Description**: If `StopControlMode()` is called while a `sendCMCommand()` caller is
blocked on its response channel, the channel will never receive a result. The `stdin`
pipe closes, the reader goroutine exits, and the pending channel is abandoned — the
caller leaks or blocks indefinitely.

**Mitigation**: `sendCMCommand()` must select on both the response channel and a
cancellation signal (either the `controlModeDone` channel or the caller's context).
When `StopControlMode()` fires, close all channels in `pendingCmds` (analogous to
how subscriber channels are closed) so blocked callers receive `cmdResult{err: ErrControlModeStopped}`.
Add cleanup of `pendingCmds` in `StopControlMode()` after closing `controlModeDone`.

**Files**: `session/tmux/control_mode.go` — `StopControlMode()` (line 83), new `sendCMCommand()`

### Data Integrity: `cmdBodyBuf` Corruption on Unexpected `%begin`

**Severity**: Medium

**Description**: If tmux emits a second `%begin` before a matching `%end` (which
should not happen per the protocol spec but can occur if the CM process is restarted
or the connection is in a partially-initialized state), the state machine enters
`inCmdResp = true` again while `cmdBodyBuf` already contains partial data from the
previous incomplete response.

**Mitigation**: When `%begin` arrives and `inCmdResp` is already `true`, log an error,
drain the current `pendingCmds` head with an error result, reset `cmdBodyBuf`, and
enter the new response block cleanly. Add a test case for this path.

**Files**: `session/tmux/control_mode.go` — `processControlModeLine()` `%begin` case

### Staleness: `IsDirty()` Cache Miss After Manual Commit

**Severity**: Low (by design — documented in ADR-002)

**Description**: The 15-second TTL means `IsDirty()` can report `true` for up to 15
seconds after a user manually commits their changes outside of Claude. The status
indicator will show "dirty" briefly after the commit.

**Mitigation**: This is an accepted trade-off (documented in ADR-002). If it causes
user confusion, add an explicit cache invalidation call in the commit/push code paths
in `session/git/worktree_git.go`.

**Files**: `session/git/worktree_git.go` — `PushChanges()`, `IsDirty()`

### Integration: `display-message` Format String Quoting Over CM Stdin

**Severity**: High (blocks Story 2b)

**Description**: The format strings passed to `display-message` contain spaces
(e.g., `"#{pane_width} #{pane_height}"`). Over a subprocess call, these are passed
as a single argument. Over CM stdin, the command is a raw text line — it is unclear
whether tmux treats the space as argument separator or parses the format string as
a single token.

**Mitigation**: Before implementing Story 2b, test interactively: open a tmux CM
connection (`tmux -C attach-session -t SESSION`), type `display-message -p "#{pane_width} #{pane_height}"`,
and observe whether the response contains two values or an error. If quoting is
required, wrap format strings in single quotes in the CM command builder.

**Files**: `session/tmux/tmux.go` — `GetPaneDimensions()` and all other `display-message` call sites

### Resource Leak: `pendingCmds` Channel Growth Under Error Conditions

**Severity**: Medium

**Description**: If `sendCMCommand()` successfully enqueues to `pendingCmds` but the
CM process exits before delivering the `%end` response, the channel in the queue is
never sent to and never closed. The goroutine waiting on it will leak until the
`context.Context` timeout fires.

**Mitigation**: In `readControlModeOutput()`, after the scanner exits, drain all
remaining `pendingCmds` channels with `cmdResult{err: ErrControlModeExited}`.
This mirrors the subscriber channel cleanup already done for `controlModeSubscribers`
at scanner EOF. Ensure this cleanup happens under `controlModeSubMu` WLock to prevent
races with concurrent `sendCMCommand()` callers that may be appending.

**Files**: `session/tmux/control_mode.go` — `readControlModeOutput()` (line 146)

### Performance: `cmdBodyBuf` Allocation Per Command

**Severity**: Low

**Description**: `bytes.Buffer` in the struct accumulates content without resetting
the underlying allocation. For sessions that call `CapturePaneContent()` frequently,
`cmdBodyBuf` will grow to the size of the largest pane capture and hold that memory
permanently.

**Mitigation**: After sending the response via `%end`, call `cmdBodyBuf.Reset()` to
retain the allocation but zero the length. Do not call `cmdBodyBuf = bytes.Buffer{}`
(reallocates) — `Reset()` is the correct pattern for buffer reuse.

**Files**: `session/tmux/control_mode.go` — `processControlModeLine()` `%end` case

---

## Context Preparation Guide

Load these files at the start of each story's implementation session. Do not load all
files upfront — load only what is listed per story.

| Story | Primary files | Supporting files |
|-------|--------------|-----------------|
| 1a | `session/git/worktree.go`, `session/git/worktree_git.go` | `session/instance.go` (IsDirty callers) |
| 1b | `github/client.go` | — |
| 1c | `session/review_queue_poller.go`, `session/instance.go` (Preview) | ADR-002 |
| 2a | `session/tmux/control_mode.go`, `session/tmux/tmux.go` (struct + GetPaneDimensions) | ADR-001 |
| 2b | `session/tmux/tmux.go` (4 functions) | output from Story 2a `sendCMCommand()` |
| 2c | `session/tmux/tmux.go` (3 functions) | tmux wiki on `%` escaping in CM body |
| 2d | `session/tmux/tmux.go` (RefreshClient) | ADR-001 consequences section |
| 3a | `session/review_queue_poller.go` | EventBus grep results |

**Fresh session rule**: Stories 2a through 2d involve tight protocol-level work.
Start a fresh session for each story to avoid carrying stale assumptions from planning
into the implementation. Stories 1a, 1b, and 1c are independent and can run in the
same session if context budget allows.
