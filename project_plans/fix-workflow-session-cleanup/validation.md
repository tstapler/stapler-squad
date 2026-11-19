# Validation: fix-workflow-session-cleanup

## Adversarial Review Summary

Reviewed plan.md for correctness, completeness, and risk.

### Confirmed correct

1. **Root cause diagnosis is accurate**: `retention.go` Phase 1 and Phase 2 both hard-code
   `entsession.StatusNotIn(Active, Creating, Paused)`. An Active session is unreachable by
   either phase. `maybeAutoArchive` skips the session when `archiveAfterHours > 0`. This is
   a confirmed gap, not a hypothesis.

2. **Skill interactive gate is real**: Step 4 of SKILL.md explicitly says "Get confirmation
   on removal scope" before any `git worktree remove`. In an unattended session, this causes
   Claude to block on AskUserQuestion with no one to answer.

3. **Phase 0 predicate is correct**: `UpdatedAtLT(cutoff)` with `StatusIn(Active, Creating)`
   is the right proxy for "stuck and not making progress." It mirrors the existing Phase 1
   predicate exactly except for the status set.

4. **`ArchivedAtIsNil()` guard prevents double-archive**: Both Phase 0 and Phase 1 use this
   guard. The ent ORM will apply it in the WHERE clause atomically.

### Open questions / risks accepted

1. **What updates `updated_at` for a running session?** If the session row is updated only
   on status transitions (not on scrollback/output), a genuinely running 24-hour session
   could be falsely archived by Phase 0. This is acceptable because: (a) workflow sessions
   are not expected to run for 24 hours legitimately, and (b) the `archive_after_hours` value
   is operator-configured — if 24h is too short, the operator can raise it.

2. **Paused sessions excluded from Phase 0 by design**: Paused is a user-initiated state.
   Including it would risk archiving a session the user is about to resume. Correct to exclude.

3. **A3 is a data/config change, not code**: Updating the workflow prompt is done via the
   UI or MCP tool. No code review required for this step.

### Missing from plan — added here

- **Metric naming**: `workflow_retention_stuck_active_archived_total` follows the existing
  `[workflows/retention]` log prefix. Confirm this matches the project's metric naming
  convention before shipping B3.

- **Test coverage for Phase 0 negative case**: B2 should include a test where the session's
  `updated_at` is recent (within threshold) — it must NOT be archived. Plan mentions this;
  verify the test implementation includes it.

## Verdict

Plan is sound. No blocking concerns. Proceed with implementation.
