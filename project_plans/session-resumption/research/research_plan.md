# Research Plan: Session Resumption

**Date**: 2026-04-03
**Input**: `project_plans/session-resumption/requirements.md`

## Subtopics

### 1. Stack
**Focus**: Technology options for file descriptor tracking, checkpoint/restore on macOS, Unix process re-parenting
**Search strategy**:
- `lsof procfs kqueue file descriptor tracking macOS Go`
- `CRIU alternatives macOS process checkpoint restore userspace`
- `Unix TTY re-parenting process adoption macOS`
- `go os/exec process file descriptor tracking`
- `macOS dtrace kqueue fsevents file open tracking`
**Output**: `research/findings-stack.md`
**Search cap**: 5

### 2. Features
**Focus**: Survey comparable tools — tmux-resurrect, tmux-continuum, Zellij, iTerm2, Claude CLI conversation continuity
**Search strategy**:
- `tmux-resurrect tmux-continuum session persistence implementation`
- `Zellij session management layout serialization`
- `iTerm2 session restoration state save`
- `Claude CLI conversation history jsonl format resume`
- `Aider session history persistence resumption`
**Output**: `research/findings-features.md`
**Search cap**: 5

### 3. Architecture
**Focus**: Ent schema for checkpoints, forking/copy-on-write session state, adopted-process bridging
**Search strategy**:
- `conversation branching fork checkpoint database schema design`
- `copy-on-write session state immutable history tree`
- `Unix PTY adoption reparenting without SIGTSTP`
- `Go ent ORM versioned entity checkpoint pattern`
- `tmux session snapshot restore architecture`
**Output**: `research/findings-architecture.md`
**Search cap**: 5

### 4. Pitfalls
**Focus**: TTY re-parenting limits on macOS, race conditions, history file locking, PID stability
**Search strategy**:
- `macOS TTY reparent ptrace limitations`
- `Go fsnotify file watch race condition JSONL append`
- `Unix history file locking concurrent write`
- `PID reuse session identity stability`
- `PTY adoption zombie process macOS`
**Output**: `research/findings-pitfalls.md`
**Search cap**: 5

## Synthesis Output
`research/synthesis.md` — combined findings across all 4 dimensions
