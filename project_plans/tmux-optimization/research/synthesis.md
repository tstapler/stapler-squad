# Research Synthesis: tmux Pipeline Optimization

## Decision Required

How to eliminate the ~138 subprocess/second hotspot (confirmed by execution trace) that causes terminal opening latency and saturates the OS process table, without introducing unmaintainable tmux internals.

## Context

Execution trace profiling showed:
- `os/exec.(*Cmd).Start.func2`: 277 goroutines = ~138 subprocess spawns/10s at idle
- `ReviewQueuePoller.pollLoop`: 57% of all CPU execution time, 5.3s/10s in syscalls
- `github.CheckGHAuth`: 2.02s cumulative mutex delay

**Surprise finding from reading actual source code**: The subprocess storm does NOT come from `gh` CLI GitHub API calls. It comes from two sources running every 2-second poll tick:
1. `tmux capture-pane` via `inst.Preview()` — for sessions without an active `ClaudeController`
2. `git status --porcelain` via `worktree.IsDirty()` — for every session with a git worktree, even when Claude is actively processing

Control mode (`tmux -C attach-session`) is already running per session for real-time streaming. The `controlModeStdin` pipe is open but unused for sending commands. This is the zero-subprocess path waiting to be wired up.

## Options Considered

| Option | Summary | Key Trade-off |
|--------|---------|---------------|
| Control mode command dispatch | Wire request/response multiplexer into existing `controlModeStdin` | Documented, proven (iTerm2 uses it), eliminates all per-query forks |
| Raw Unix socket IPC | Speak tmux's binary `imsg` protocol directly | Private, undocumented, changes between releases — 2–4 weeks to implement, breaks on tmux upgrades |
| Replace subprocess with go-git/libgit2 | Use Go library for `git status` instead of subprocess | libgit2 is 2.5–6x **slower** than `git status` subprocess — wrong direction |
| TTL cache + adaptive polling | Cache `IsDirty()` and `Preview()` results, slow poll when idle | Quickest fix, independent of control mode work, high ROI |
| Replace `gh` CLI with `google/go-github` | HTTP API calls instead of subprocess for GitHub operations | Moderate effort; blocked on confirming it's actually the bottleneck (it isn't the primary one) |

## Dominant Trade-off

**Completeness vs. implementation risk.** The control mode command dispatch eliminates the root cause (subprocess per query) cleanly. The raw socket IPC approach eliminates the same forks but adds protocol stability risk that is not justified — the one persistent control mode process it additionally eliminates is already running for streaming and cannot be removed. TTL caching is the fastest win and can ship independently.

The two approaches are **complementary, not competing**: cache first (days), then wire control mode commands (1 week).

## Recommendation

**Choose: TTL caching + control mode command dispatch in two phases.**

**Because**: The subprocess forks from `CapturePaneContent()` and `IsDirty()` are the confirmed bottleneck (not `gh` CLI). TTL caching for `IsDirty()` (15s TTL, skip when Claude active) and `Preview()` (500ms TTL) eliminates the majority of forks in 1–2 days with zero protocol risk. Control mode command dispatch then eliminates the remainder via the already-open `controlModeStdin` pipe using a documented, stable protocol that iTerm2 and other production terminals rely on.

**Accept these costs**:
- Control mode command dispatch adds a state machine to `processControlModeLine()` (~200–350 LOC) and requires fallback-to-subprocess when CM is not running
- TTL caching adds staleness — `IsDirty()` at 15s TTL may miss a commit for up to 15s; acceptable for a status indicator

**Reject these alternatives**:
- **Raw Unix socket IPC**: rejected — tmux's `imsg` binary protocol is private, undocumented, breaks between releases, and offers no benefit beyond the one persistent control-mode process it eliminates (which must exist anyway for streaming)
- **go-git/libgit2 for `IsDirty()`**: rejected — benchmarks show libgit2 is 2.5–6x slower than `git status` subprocess; would make the problem worse
- **GitHub webhooks for ReviewQueuePoller**: rejected — operational complexity not justified for a local developer tool; polling with adaptive intervals is sufficient

## Implementation Phases

### Phase 1: Caching (1–2 days, high ROI, zero protocol risk)

**1a. `IsDirty()` TTL cache** (`session/git/`)
- Add `isDirtyCache bool` + `isDirtyCacheTime time.Time` + `isDirtyCacheTTL = 15s` to `GitWorktree`
- Skip entirely when `ClaudeController` reports active (already have that signal)
- Expected reduction: eliminates ~50% of subprocess forks

**1b. `CheckGHAuth` singleflight + atomic TTL** (`github/client.go`)
- `var authState atomic.Value` storing `{ok bool, expiry time.Time}`
- `var authGroup singleflight.Group`
- Check atomic first; on expiry, call `authGroup.Do("auth", checkFn)`
- TTL: 5 minutes — auth token changes are rare
- Expected reduction: eliminates 2.02s cumulative mutex delay

**1c. `Preview()` result cache** (`session/instance.go`)
- 500ms TTL on the `capture-pane` result used by `ReviewQueuePoller`
- Already partially cached via `ClaudeController` activity cache — extend to non-controller sessions

### Phase 2: Control Mode Command Dispatch (3–5 days)

Wire the existing `controlModeStdin` pipe to handle command/response cycles, eliminating all remaining `tmux` subprocess forks for query operations.

**Protocol** (confirmed from tmux wiki):
```
stdin:  capture-pane -p -e -t staplersquad_mysession\n
stdout: %begin <unix_time> <cmd_num> 0
        ... pane content lines ...
        %end <unix_time> <cmd_num> 0
```
tmux guarantees sequential ordering ("will never mix output for different commands") — a simple FIFO channel queue is sufficient, no MSGID map needed.

**New struct fields** on `TmuxSession`:
```go
pendingCmds  []chan cmdResult  // FIFO queue, protected by controlModeSubMu
cmdBodyBuf   bytes.Buffer      // accumulates lines between %begin and %end
inCmdResp    bool              // state machine flag
```

**Call sites to migrate** (9 total, in priority order):
1. `GetPaneDimensions()` — small output, easy to verify first
2. `GetCursorPosition()` — same pattern as above
3. `GetPaneCurrentPath()` — same pattern
4. `GetPanePID()` — same pattern
5. `CapturePaneContent()` — larger output, verify state machine handles multi-line correctly
6. `CapturePaneContentRaw()` — same as above
7. `CapturePaneContentWithOptions()` — includes `-S`/`-E` flags (confirmed to work in CM)
8. `SetWindowSize()` — fire-and-forget resize command
9. `RefreshClient()` — fire-and-forget; verify behavior when CM is the target client

**Rollout**: feature flag `STAPLER_SQUAD_CM_COMMANDS=true`, start with `GetPaneDimensions`, run both paths in parallel for 24h logging discrepancies, then migrate remaining 8 functions.

### Phase 3: ReviewQueuePoller Adaptive Interval (1 day, after Phase 1)

Wire `EventBus` into `ReviewQueuePoller`: back off to 8s interval when no sessions are awaiting approval; snap to 2s on `EventApprovalResponse` or `EventUserInteraction`. Reduces baseline subprocess rate by ~4x for the idle case.

## Open Questions Before Committing

- [ ] Verify `display-message` format string quoting over CM stdin: does `"#{pane_width} #{pane_height}"` parse correctly without shell quoting?
- [ ] Verify `refresh-client -t SESSION` behavior when sent from the CM connection that is itself attached to that session
- [ ] Confirm `capture-pane -S/-E` line range flags work over CM stdin (probable yes, not yet tested)

These are verification questions, not blockers — Phase 1 caching can ship while Phase 2 is being validated.

## Sources

- [findings-control-mode-commands.md](findings-control-mode-commands.md) — protocol details, implementation design, web search verification
- [findings-tmux-socket-ipc.md](findings-tmux-socket-ipc.md) — socket IPC analysis and rejection rationale
- [findings-review-queue-poller.md](findings-review-queue-poller.md) — root cause identification, caching strategies
- [tmux Control Mode Wiki](https://github.com/tmux/tmux/wiki/Control-Mode) — protocol specification
- [singleflight](https://victoriametrics.com/blog/go-singleflight/) — Go deduplication pattern
- [libgit2 performance issue](https://github.com/libgit2/libgit2/issues/4230) — why go-git is not the answer
