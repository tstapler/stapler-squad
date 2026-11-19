# Plan: fix-workflow-session-cleanup

## Overview

Two coordinated changes: (1) add an unattended-mode flag to the worktree-disk-cleanup skill
that pre-authorizes Safe-bucket deletion without pausing for confirmation, and (2) extend
the workflow retention enforcer to detect and archive sessions that have been stuck Active
beyond a stale threshold, using the existing `updated_at` column as the staleness proxy.

## Task Breakdown

### Track A — Skill: unattended mode

**A1** — Add `UNATTENDED=true` / `--unattended` prose flag to `worktree-disk-cleanup/SKILL.md`
Step 4. When this flag is present in the prompt:
- Skip the interactive confirmation gate for the **Safe** bucket only.
- Log each deletion decision to stdout before executing it ("Auto-removing Safe-bucket worktree
  <path>: MERGED PR #N").
- Still gate on Manual-review and Keep buckets — emit a human-readable summary and stop without
  deleting.

Exact location: `.claude/skills/worktree-disk-cleanup/SKILL.md`, Step 4 section.
~30 lines added to the skill prose; no Go code changed.

**A2** — Document in a "Scheduling / automation" subsection at the bottom of the skill that
when this skill is invoked from an unattended scheduled workflow, the prompt MUST include the
unattended-mode instruction. Example workflow prompt snippet included.

**A3** — Update the `worktree-disk-cleanup-nightly` workflow's command/prompt (in the UI or
via MCP `update_workflow`) to include the unattended flag instruction.
This is a data change, not a code change.

### Track B — Retention enforcer: stuck-Active timeout

**B1** — Extend `WorkflowRetentionEnforcer`'s sweep to add a Phase 0 (before Phase 1):

```go
// Phase 0: archive workflow sessions stuck Active for longer than archive_after_hours.
// Proxy: UpdatedAt hasn't moved in archive_after_hours hours (same cutoff as Phase 1).
// Only applies when archive_after_hours > 0.
if wf.ArchiveAfterHours > 0 {
    cutoff := now.Add(-time.Duration(wf.ArchiveAfterHours) * time.Hour)
    n, err := entClient.Session.Update().
        Where(
            entsession.WorkflowID(wf.ID.String()),
            entsession.ArchivedAtIsNil(),
            entsession.StatusIn(int(domain.Active), int(domain.Creating)),
            entsession.UpdatedAtLT(cutoff),
        ).
        SetArchivedAt(now).
        Save(ctx)
    if err != nil {
        log.Warn("[workflows/retention] stuck-active archive failed", ...)
    } else if n > 0 {
        log.Warn("[workflows/retention] archived stuck-Active sessions",
            "workflow", wf.Slug, "count", n, "cutoff", cutoff)
        archived += n
    }
}
```

This deliberately uses a `Warn`-level log on archival so the operator can see it in
`logs/staplersquad.log` and investigate why sessions were stuck.

Paused sessions are intentionally excluded from Phase 0 — a Paused session is a user-initiated
state, not a runaway.

**B2** — Add a unit test in `server/workflows/retention_test.go` that:
- Creates a workflow with `archive_after_hours: 24` and `keep_sessions: 1`.
- Creates one session in Active status with `updated_at` = 25 hours ago.
- Runs `RunRetentionSweep`.
- Asserts the session is now archived.
- Verifies the previously-safe session (updated 23 hours ago) is NOT archived.

**B3** — Add a metric counter `workflow_retention_stuck_active_archived_total` (labels:
`workflow_slug`) so operators can alert on this in Grafana.

### Track C — Docs

**C1** — Add entry to `docs/explanation/autonomous-stall-causes.md` for the interactive-
confirmation stall (AskUserQuestion in an unattended workflow) and reference the new unattended
flag as the fix.

## Adversarial Review

**Risk: Phase 0 archives a genuinely running session**

A session that is truly running a long Bazel build or git operation could have a stale `updated_at`
if the session row isn't updated during long background work. Mitigation: `archive_after_hours: 24`
is a generous threshold; a Bazel build running for 24+ hours without any state transition is an
unlikely legitimate case. Additionally, only workflow-spawned sessions are targeted — not
manually-created sessions. Acceptable risk.

**Risk: unattended flag deletes a worktree that has real work in the Safe bucket**

The Safe bucket already requires MERGED or CLOSED PR + only build-junk dirty files. A worktree
that lands in this bucket genuinely has no unshipped work by definition. The classification
logic (Step 2) is unchanged. The unattended flag only skips the human confirmation, not the
classification. Low risk.

**Risk: Phase 0 and `maybeAutoArchive` double-archive**

`archiveExcessSessions` already uses `entsession.ArchivedAtIsNil()` as a guard. Phase 0 uses
the same guard. `SetArchivedAtIfNil` is idempotent. No double-archive is possible.

**Risk: skill changes break interactive use**

The unattended flag is opt-in. If the prompt does not include it, behavior is unchanged.
Existing interactive callers are unaffected.

## Estimated Effort

| Task | Estimate | Category |
|------|----------|----------|
| A1: skill unattended mode (prose) | 1h | docs |
| A2: scheduling doc in skill | 30m | docs |
| A3: update workflow prompt (data) | 15m | infra |
| B1: Phase 0 in retention enforcer | 2h | backend |
| B2: retention unit test | 1.5h | test |
| B3: metric counter | 1h | backend |
| C1: autonomous-stall-causes doc update | 30m | docs |

Total: ~7 hours
