# ADR-004: Lazy Git Enrichment for ClaudeHistoryEntry Fields

**Status**: Accepted
**Date**: 2026-04-12
**Project**: history-page-revamp

## Context

Rich session cards require additional fields not currently in `ClaudeHistoryEntry`: git branch, session lifecycle status, git diff count, and last commit message. These fields must be populated without blowing the 200ms initial render budget.

The history cache is built by scanning all JSONL files on disk. Adding git I/O inside `parseConversationFile` would multiply disk latency (one `git` operation per JSONL file on every reload).

## Decision

Populate new `ClaudeHistoryEntry` fields lazily, with per-field cost analysis determining whether each is computed eagerly, lazily, or not at all during the list endpoint:

| Field | Population strategy | Cost |
|---|---|---|
| `session_status` | Always, in-memory lookup | O(1) map lookup against session store |
| `branch` | Cached per project path, 60s TTL | One `exec.Command` per unique project path per cache cycle |
| `git_status_summary` | Only when a live stapler-squad session with `resume_id == entry.id` exists | `go-git Worktree().Status()` — zero cost for 99% of entries |
| `last_commit_message` | Same condition as git_status_summary | `go-git Head() + CommitObject()` |
| `diff_file_count` | Same call as git_status_summary | Counted from Status() result |

## New Proto Fields

```protobuf
message ClaudeHistoryEntry {
  // ... existing fields 1–7 unchanged ...
  string branch             = 8;  // git branch name; empty if unknown
  string git_status_summary = 9;  // e.g. "2 modified, 1 untracked"; empty if no live worktree
  SessionStatus session_status = 10; // lifecycle status from session store
  string last_commit_message   = 11; // HEAD commit subject line; empty if no live worktree
  int32  diff_file_count        = 12; // files changed vs HEAD; -1 = not available
}
```

## Branch Cache Design

```go
type branchCache struct {
    mu      sync.Mutex
    entries map[string]branchCacheEntry  // key: project path
}

type branchCacheEntry struct {
    branch    string
    expiresAt time.Time
}
```

A miss triggers `exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")` in the project directory, cached for 60 seconds. This is safe from shell injection because path is passed as an argument, not interpolated into a shell string.

## go-git vs exec.Command

For `branch` and `last_commit_message`: prefer `go-git` (already in `go.mod`, no subprocess overhead, no shell injection risk).

For `git_status_summary`: `go-git Worktree().Status()` is known to be slow on large repos (linear scan of all tracked files). If it takes >200ms, fall back to `exec.Command("git", "status", "--short")` with a 2-second timeout. Only triggered when a live session worktree exists.

## Consequences

**Positive:**
- `ListClaudeHistory` latency unchanged for the 99% case (no live worktrees)
- `session_status` is always accurate (in-memory, no I/O)
- Branch data appears after first cache warm-up (60s max staleness is acceptable for a history browser)

**Negative:**
- `git_status_summary` and `diff_file_count` are empty for historical sessions with no live worktree (acceptable — these fields are most useful for active/recently-paused sessions)
- Branch cache adds ~200 bytes per unique project path — negligible

## Patterns Applied

- **Lazy loading** — expensive data computed on demand
- **Cache-aside** — populate cache on miss, serve from cache on hit
- **Null object** — empty string / -1 sentinel for unavailable fields, not error responses

## Related

- `session/history.go` — ClaudeHistoryEntry struct
- `server/services/search_service.go` — ListClaudeHistory handler (branch cache lives here)
- `session/instance.go` — SessionStatus enum source of truth
- research/architecture.md §1 — full field sourcing table
