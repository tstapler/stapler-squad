# Research Plan: tmux Pipeline Optimization

## Problem Statement

Profiling shows ~138 subprocess spawns/second from `os/exec.(*Cmd).Start.func2` goroutines.
The codebase already uses tmux control mode for real-time streaming (`readControlModeOutput`)
but still fires individual subprocesses for every: `capture-pane`, `display-message`,
`resize-window`, `list-sessions`, `refresh-client`, `GetPaneDimensions`, `GetCursorPosition`,
`GetPaneCurrentPath`, `GetPanePID`, and `HasUpdated` call.

Key code: `session/tmux/tmux.go`, `session/tmux/control_mode.go`

The control mode stdin pipe (`controlModeStdin`) is open but unused for sending commands.
The tmux control mode protocol allows sending commands over stdin and receiving structured
`%begin`/`%end` delimited responses — this is the zero-subprocess path.

## Subtopics

### 1. control-mode-commands (HIGH VALUE)
**Question**: Can we send `capture-pane`, `display-message`, `resize-window` etc. over the
existing control mode stdin pipe and get responses back via `%begin`/`%end`, eliminating
all `tmux` subprocess spawns for querying operations?

**Search cap**: 4 searches
**Key axes**: protocol correctness, response parsing complexity, concurrency safety,
coverage of all current subprocess call sites
**Output**: `findings-control-mode-commands.md`

### 2. tmux-socket-ipc (MEDIUM VALUE)
**Question**: Can we speak the tmux Unix domain socket protocol directly from Go (without
spawning a subprocess at all) to eliminate even the control mode attach process?

**Search cap**: 3 searches
**Key axes**: protocol complexity, maintenance burden vs subprocess approach, portability,
Go library availability
**Output**: `findings-tmux-socket-ipc.md`

### 3. review-queue-poller-optimization (HIGH VALUE)
**Question**: The `ReviewQueuePoller.pollLoop` consumes 57% of all CPU execution time
(5.3s/10s in syscalls) running what appear to be `gh` CLI calls. What are the best patterns
for caching/batching GitHub API calls to reduce subprocess frequency by 10x?

**Search cap**: 4 searches
**Key axes**: `gh` CLI vs GitHub REST/GraphQL API directly, caching TTL strategy,
singleflight coalescing, webhook vs polling
**Output**: `findings-review-queue-poller.md`
