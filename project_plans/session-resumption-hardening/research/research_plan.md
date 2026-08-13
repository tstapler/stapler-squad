# Research Plan: Session Resumption Hardening

**Date**: 2026-04-15
**Requirements input**: `project_plans/session-resumption-hardening/requirements.md`

## Subtopics

### 1. Stack — Current codebase state audit
**Goal**: Verify which "❓ Verify" items are stubs vs real implementations. Determine exactly what is missing vs partially built.
**Method**: Codebase read (Grep/Read/Glob) — no web search needed
**Files to audit**:
- `server/dependencies.go` or equivalent server startup wiring
- `session/checkpoint.go` — CreateCheckpoint method completeness
- `server/services/session_service.go` — checkpoint RPC handlers
- `server/adapters/instance_adapter.go` — proto field population
- `session/instance.go` — CaptureCurrentState, cold restore branch in Start()
- `session/scrollback/storage.go` — LatestSequence method
**Search cap**: N/A — codebase reads, not web search
**Trade-off axes**: Completeness, test coverage, wiring correctness
**Output**: `research/findings-stack.md`

### 2. Features — Checkpoint UX survey
**Goal**: Understand how comparable tools handle checkpoint/bookmark UX to inform label+list UI design
**Method**: Training knowledge + web search for recent patterns
**Candidates**: tmux-resurrect, VSCode workspace restore, JetBrains session save, git stash UI patterns, Linear issue bookmarks
**Search cap**: 4 searches
**Searches**:
1. `tmux resurrect bookmark session checkpoint UX` — tmux precedent
2. `VSCode workspace restore session management 2025` — IDE checkpoint patterns
3. `checkpoint UI design bookmark session list` — general UI patterns
4. `git stash named stash UX design` — named-point-in-time metaphor
**Trade-off axes**: Discoverability, naming friction, list readability, action reversibility
**Output**: `research/findings-features.md`

### 3. Architecture — Missing wiring design
**Goal**: Design exactly where/how HistoryLinker starts, how CaptureCurrentState fits into shutdown, and how the graceful shutdown sequence chains together
**Method**: Codebase read (server startup, shutdown, dependencies) + training knowledge on Go context cancellation patterns
**Files to read**:
- `main.go` — serve subcommand, shutdown sequence
- `server/server.go` — Shutdown() method
- `server/dependencies.go` — what's already constructed at startup
- `session/history_linker.go` — Start() signature and what it needs
- `session/instance.go` — Start(false) cold restore branch
**Search cap**: 2 searches
**Searches**:
1. `Go graceful shutdown context cancellation cleanup hooks pattern` — shutdown sequencing
2. `Go dependency injection startup wiring background goroutines pattern` — startup wiring
**Trade-off axes**: Startup ordering correctness, shutdown race-freedom, testability
**Output**: `research/findings-architecture.md`

### 4. Pitfalls — Known failure modes
**Goal**: Catalog concrete risks in the implementation: races, gopsutil CGo on CI, scrollback contention during checkpoint creation, cold restore to deleted path
**Method**: Codebase read of existing KI-NNN entries in plan.md + training knowledge on Go race patterns
**Known issues to expand**:
- KI-001: gopsutil CGo requirement
- KI-003: PID reuse during history correlation
- KI-004: Partial JSONL line during concurrent read
- KI-007: Cold restore with missing WorkingDir
- New: race between HistoryLinker startup scan and session serve
- New: scrollback LatestSequence read while session is writing
**Search cap**: 2 searches
**Searches**:
1. `gopsutil CGo github actions macOS linux cross-compile 2025` — CI build risk
2. `Go test race fsnotify concurrent file creation` — watcher test races
**Trade-off axes**: Severity (data loss vs degraded UX), detectability, mitigation cost
**Output**: `research/findings-pitfalls.md`

## Parallelization

All 4 subtopics are independent and can run simultaneously. Spawn 4 agents in parallel.

## Synthesis

After all 4 findings files complete:
- Parent agent runs all pending web searches
- Parent writes `research/synthesis.md` in ADR-Ready format
- Synthesis feeds directly into `/plan:feature` (Phase 3)
