# Requirements: fix-workflow-session-cleanup

## Problem Statement

The "Nightly Worktree Disk Cleanup" scheduled workflow (`worktree-disk-cleanup-nightly`,
configured: Daily at 3 AM, `keep_sessions: 1`, `archive_after_hours: 24`) accumulates open
sessions instead of cleaning them up. The workflow spawns a Claude session each night, but
those sessions never reach a Stopped state — they stay Active indefinitely — so the
retention enforcer never archives them.

## Root Cause Analysis

**Two compounding issues:**

### 1. Skill has interactive confirmation gates (primary)

The `worktree-disk-cleanup` skill's Step 4 explicitly requires human confirmation before any
`git worktree remove`:

> "Get confirmation on removal scope (which buckets) before running any `git worktree
> remove`, even when every entry in a bucket looks safe — this is a bulk destructive
> operation over other people's (or your own past sessions') work."

When invoked from an unattended scheduled workflow, Claude reaches this gate, asks for
confirmation, and waits. No human is present to respond. The session stays Active until
the server restarts (or indefinitely).

### 2. Retention enforcer never touches Active sessions (secondary)

`server/workflows/retention.go` Phase 1 and Phase 2 both filter out sessions with status
Active (1), Creating (0), or Paused (2):

```go
entsession.StatusNotIn(
    int(domain.Active),
    int(domain.Creating),
    int(domain.Paused),
),
```

Sessions stuck Active bypass `keep_sessions` enforcement entirely. With `archive_after_hours: 24`,
`maybeAutoArchive` also defers to the retention enforcer — meaning there is no path that
archives a workflow session that never Stops.

## User Story

As an operator running nightly worktree cleanup workflows, I want session cleanup to work
unattended — sessions should complete, and old sessions should be archived automatically —
so the workflow doesn't accumulate indefinitely open sessions that consume UI space and
confuse the session list.

## Acceptance Criteria

1. **Unattended mode in skill**: The `worktree-disk-cleanup` skill supports an "unattended"
   / "auto-approve" mode flag in its Step 4 that pre-authorizes deletion of Safe-bucket
   worktrees (MERGED/CLOSED PR, build-junk-only dirty) without pausing for confirmation.

2. **Workflow prompt documents unattended flag**: The workflow's command/prompt includes
   the unattended-mode instruction so Claude knows not to pause for confirmation.

3. **Retention enforcer handles stuck-Active workflow sessions**: The retention enforcer
   gains an optional `stuck_active_timeout_hours` per-workflow setting (or uses a multiple
   of `archive_after_hours`). When a workflow session has been Active for longer than the
   timeout with no terminal activity recorded, the enforcer force-archives it and emits a
   warning log.

4. **Keep-sessions enforcement applies after force-archive**: After the stuck-Active archival
   pass, the `keep_sessions` count-based phase runs and correctly prunes excess sessions.

5. **No existing safe-bucket worktrees are auto-deleted without warning**: Unattended mode
   still logs every deletion decision and what it found in the session's log output.

6. **Regression test**: A unit test verifies that the retention enforcer archives a session
   that has been Active for longer than the configured stuck timeout.

## Out of Scope

- Fixing the worktree-disk-cleanup skill's Step 3 (individual review agents) — those
  already run non-interactively.
- Changing the confirmation requirement for Manual/KEEP bucket worktrees — those should
  still require human approval; only the Safe bucket is pre-authorized.
- Automated branch deletion (explicitly deferred in the skill's Notes).
