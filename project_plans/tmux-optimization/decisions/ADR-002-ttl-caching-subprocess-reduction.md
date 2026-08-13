# ADR-002: TTL caching for subprocess-heavy polling operations

**Status**: Proposed
**Date**: 2026-04-24
**Project**: tmux-optimization

## Context

The `ReviewQueuePoller.pollLoop` consumes 57% of all CPU execution time (5.3s/10s in syscalls) and is the primary driver of the ~138 subprocess/second hotspot. Profiling identified three sources that fire on every 2-second poll tick regardless of whether results have changed:

1. **`worktree.IsDirty()`** — runs `git status --porcelain` for every session with a git worktree, including sessions where `ClaudeController` is actively processing (Claude never changes worktree state during active generation)
2. **`inst.Preview()`** — runs `tmux capture-pane` for every session without an active `ClaudeController` (even when the terminal hasn't changed)
3. **`CheckGHAuth()`** — runs `gh auth status` and holds a shared mutex during the check; profiling shows 2.02s cumulative mutex delay

ADR-001 (control mode command dispatch) eliminates the subprocess forks from these calls in Phase 2. This ADR addresses Phase 1: reducing call frequency with TTL caching while Phase 2 is built and validated.

Two replacement approaches were evaluated for `IsDirty()`: TTL caching vs. replacing with a Go library.

## Decision

We decided to add TTL caching at three callsites, each with a TTL calibrated to its data volatility:

**`IsDirty()` — 15-second TTL, skip when Claude is active**
- Add `isDirtyCache bool`, `isDirtyCacheTime time.Time`, `isDirtyCacheTTL = 15s` to `GitWorktree`
- Skip the call entirely (return cached value or `false`) when `ClaudeController` reports active — Claude never commits or modifies worktree state mid-generation
- Expected reduction: ~50% of subprocess forks

**`CheckGHAuth()` — 5-minute singleflight + atomic TTL**
- Replace shared mutex with `var authState atomic.Value` storing `{ok bool, expiry time.Time}`
- Use `var authGroup singleflight.Group` to coalesce concurrent expiry checks
- Check atomic first; on expiry, call `authGroup.Do("auth", checkFn)` and update atomic
- Expected reduction: eliminates 2.02s cumulative mutex delay per profiling window

**`Preview()` — 500ms TTL**
- Cache the `capture-pane` result for sessions without an active `ClaudeController`
- Extend the existing activity cache already used by `ClaudeController` sessions to non-controller sessions

## Alternatives Considered

- **Replace `IsDirty()` with go-git or libgit2**: Rejected. Benchmarks show libgit2 is 2.5–6x slower than `git status --porcelain` for dirty-check workloads. go-git also has known performance issues (GitHub issue libgit2/libgit2#4230). Replacing the subprocess with a library would make the hotspot worse, not better.

- **GitHub webhooks for ReviewQueuePoller**: Rejected. Adds server infrastructure (public endpoint, ngrok/tunnel, webhook registration) for a local developer tool. Adaptive polling intervals (Phase 3) achieve similar idle-state reduction with zero operational overhead.

- **Increase poll interval globally**: Rejected. A longer fixed interval degrades responsiveness for the active case (when Claude is finishing and user is waiting for approval). TTL caching and adaptive intervals (Phase 3) handle the idle and active cases independently.

## Rationale

TTL caching is the fastest win: it ships in 1–2 days with zero protocol risk, is fully independent of ADR-001, and addresses the confirmed primary hotspot directly. The 15s TTL for `IsDirty()` is acceptable staleness for a status indicator — the worst-case scenario is missing a commit display for up to 15 seconds. The `skip when Claude active` optimization is zero-cost correctness: Claude cannot change worktree state during generation, so the result is deterministically stable.

The singleflight pattern for `CheckGHAuth()` eliminates the mutex bottleneck entirely — concurrent callers coalesce into one actual auth check rather than queuing behind a lock.

## Consequences

**Positive:**
- Eliminates majority of redundant subprocess forks without any protocol changes
- Ships independently of ADR-001 — can be in production while Phase 2 is developed
- `singleflight` for `CheckGHAuth()` eliminates a measured 2.02s mutex delay per profiling window
- `IsDirty()` skip-when-active is a correctness guarantee, not just an optimization

**Negative / Risks:**
- `IsDirty()` at 15s TTL may show stale dirty state for up to 15s after a manual commit outside of Claude — acceptable for a status indicator, not for a blocking gate
- `Preview()` at 500ms TTL means terminal content may lag by up to 500ms in the web UI for non-active sessions — acceptable for the review queue display
- Adds cache invalidation state to `GitWorktree` struct (minor complexity increase)

**Follow-up work:**
- Instrument cache hit rate in logs to verify expected ~50% reduction in `IsDirty()` calls
- Phase 3 (ADR-003 candidate): wire EventBus into `ReviewQueuePoller` for adaptive interval — 8s when idle, snap to 2s on `EventApprovalResponse` or `EventUserInteraction`

## Related

- Research: `project_plans/tmux-optimization/research/findings-review-queue-poller.md`
- Synthesis: `project_plans/tmux-optimization/research/synthesis.md`
- Source: `session/git/` (`GitWorktree`, `IsDirty()`)
- Source: `session/instance.go` (`Preview()`)
- Source: `github/client.go` (`CheckGHAuth()`)
- Bug: `docs/bugs/open/BUG-021-check-gh-auth-mutex-contention.md`
- Supersedes: (none)
- Related ADRs: ADR-001 (control mode command dispatch — Phase 2 follow-on)
