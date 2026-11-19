# Research: fix-workflow-session-cleanup

## Codebase Findings

### Retention enforcer (`server/workflows/retention.go`)

Runs hourly via `StartRetentionEnforcer` (server.go:905). Two phases:
- **Phase 1** (time-based): archives sessions with `UpdatedAtLT(now - archive_after_hours)` for all
  non-Active/Creating/Paused sessions of the workflow. Status guard hard-coded.
- **Phase 2** (count-based): queries non-Active/Creating/Paused sessions ordered by `CreatedAt`,
  archives oldest beyond `keep_sessions` count.

Both phases share the same status exclusion predicate — there is no path that touches
Active sessions.

### maybeAutoArchive (`server/services/session_service_lifecycle.go:1064`)

Called on session exit. If `archiveAfterHours > 0`, returns immediately (defers to retention
enforcer). The comment says "retention enforcer will archive this after the configured delay."
This is correct for sessions that stop — but if the session never stops, neither path fires.

### workflowMetaCache (`session_service_events.go:413–427`)

A short-lived in-memory cache of `{archiveAfterHours}` keyed by workflow ID, populated from
the workflow repo. No `keepSessions` value is cached here (only used in the retention enforcer
itself). Cache refresh interval not shown in the grep results but it's separate from retention.

### Interactive confirmation in skill

`SKILL.md` Step 4 explicitly says:
> "Get confirmation on removal scope (which buckets) before running any git worktree remove,
> even when every entry in a bucket looks safe"

This confirmation was added to prevent bulk automated deletion of real unshipped work. It is
load-bearing for safety in interactive use, but it is incompatible with unattended scheduling.

The fix must be opt-in and narrow: only pre-approve the Safe bucket (MERGED/CLOSED PR, only
build-junk dirty files). Manual-review and Keep buckets must still block.

### Session stuck-Active detection: existing art

`StaleSessionNotifier` (server.go:1553–1560) sends notifications for sessions idle beyond a
threshold. It does NOT archive or force-stop sessions — only notifies. It fires for all sessions,
not just workflow ones.

There is no existing mechanism to force-archive a stuck-Active workflow session. The gap is
intentional (the enforcer says "never archives Active sessions") but the rationale was correctness
(don't archive a genuinely running session), not an argument against a timeout-based fallback
for workflow sessions.

### Domain status values (from retention.go comment)

```
Creating=0, Active=1, Paused=2, Stopped=3, Hibernated=4
```

A session that Claude is in mid-turn is Active=1. A session waiting at AskUserQuestion is
still Active=1 from the system's perspective (the tmux process is running, waiting for input).

## Related Prior Art

- `docs/explanation/autonomous-stall-causes.md` documents why autonomous sessions stop early.
  The interactive-confirmation stall is not listed there as a known cause (it was deliberate
  safety behavior, not a bug), so this is a gap.
- `session/health.go` and `StaleSessionDetection` detect but don't auto-terminate.
- `noarchivedrevival.md` shows prior work around reviving archived sessions inadvertently.

## Decision

The dual fix (skill unattended mode + retention enforcer stuck-Active timeout for workflow
sessions) is the right approach. The skill fix unblocks the happy path; the retention
enforcer fix is a safety net for any future case where a workflow session gets stuck for
any reason.

The stuck-Active timeout should NOT be a global default. It should be per-workflow via a
`stuck_active_timeout_hours` field (or derived as `2 * archive_after_hours` if unset).
Archiving a stuck-Active session that is still genuinely running would be incorrect —
this must be gated on a heuristic (no terminal output in N hours) rather than pure wall-clock.

Actually, re-examining: checking for "no terminal output in N hours" requires reading the
scrollback or tracking last-output timestamp — heavier than a pure DB-side check. A simpler
proxy: use `updated_at` on the session row (the same field the time-based Phase 1 uses).
An Active session that hasn't had its `updated_at` touched in `archive_after_hours` hours
is a reasonable proxy for a stuck session. This aligns with the existing Phase 1 predicate.
