# ADR-022: WorktreePRPoller Extends PRStatusPoller, Not Replaces It

**Status**: Accepted
**Date**: 2026-06-24

## Context

`PRStatusPoller` currently accepts a feed of `[]Instance` (active sessions) to determine which PRs to poll. The github-work-continuity feature introduces a new source of PR-relevant work: git worktrees that have an associated GitHub PR but no active Stapler Squad session (e.g., a branch checked out and abandoned, or a PR opened from a worktree that was never wrapped in a session).

Two approaches were considered:

1. **Separate poller**: Introduce a dedicated `WorktreePRPoller` that independently queries GitHub for worktree-associated PRs, maintains its own ETag cache, and runs its own polling loop.

2. **Extend existing poller**: Teach `PRStatusPoller` to accept a second feed — `[]unfinished.ScanResult` — in addition to `[]Instance`. Both feeds contribute entries to a single unified index. One polling loop, one ETag cache, one rate-limit budget.

The codebase already has one polling loop and one ETag cache for session-based PRs. Duplicating this infrastructure would double the GitHub API call volume and introduce two independent places to tune polling intervals, handle backoff, and manage ETag headers.

## Decision

Rather than a separate poller, `PRStatusPoller` is extended to accept a feed of `[]unfinished.ScanResult` alongside the existing `[]Instance` feed.

Internally, both feeds are merged into a single `map["owner/repo/branch"]PRInfo` index before each poll cycle. The polling loop, ETag cache, and rate-limit backoff logic are unchanged and operate over the merged key set. The existing session-based polling path is preserved without modification — callers that provide only `[]Instance` continue to work as before.

The index key format `"owner/repo/branch"` is chosen because it is the natural join key for both `Instance` (which carries a session's associated branch and repo) and `ScanResult` (which carries the worktree's head branch and resolved remote).

## Consequences

### Positive
- One polling loop, one ETag cache, one rate-limit budget covers both sessions and bare worktrees; total GitHub API call volume is not increased by the new feature.
- Existing session-based callers are unaffected; the new `ScanResult` feed is additive and optional.
- Deduplication is free: if a worktree is both a `ScanResult` and has a corresponding `Instance`, the same index key is written once — no duplicate poll.
- Interval tuning, backoff logic, and ETag handling are maintained in a single location.

### Negative / Risks
- The `PRStatusPoller` struct gains a second input channel, increasing its surface area. Future contributors must understand that the index is built from two sources.
- If the `ScanResult` feed produces a different set of keys on each call (e.g., because worktrees are added/removed frequently), the poller's ETag cache may be invalidated more often than under the session-only model, slightly increasing API call volume.
- The `"owner/repo/branch"` key assumes branch names are unique within a repo. If two worktrees track the same branch in the same repo, their entries collide in the index — the last writer wins.

### Mitigations
- The struct comment on `PRStatusPoller` is updated to document both input feeds and the unified index, making the multi-source design explicit for future contributors.
- Branch-name collisions within the same repo are unlikely in practice (two worktrees on the same branch serve no purpose) and are documented as a known limitation; they do not cause data loss, only redundant polling.
- The ETag cache key includes the full index key set hash so that additions/removals to the tracked set correctly invalidate cached responses.
