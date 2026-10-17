# Validation: fix-triage-artifact-path

## Adversarial Review

### Challenge 1: Does Task 1's fix preserve the prior good value on re-triage failure?

**Concern:** If an item was triaged successfully (PlanArtifactsPath = `<W1>/impl`), plan approved, item queued — and then re-triage is triggered and fails to create `implementation/` in worktree W2 — does the existing good path from W1 get cleared?

**Answer:** No. The `BacklogItemUpdate.PlanArtifactsPath` field is `*string`. Passing `nil` means "don't update." The repo layer (`ent_repository_backlog.go:1226`) only calls `SetPlanArtifactsPath` when `update.PlanArtifactsPath != nil`. The fix only sets the pointer when `os.Stat` succeeds; on failure it leaves it as `nil`, preserving the prior good value in the DB. This is correct behavior: a failed re-triage should not overwrite a valid prior path.

**Verdict:** Fix is safe. The prior good path is preserved on re-triage failure.

---

### Challenge 2: Does Task 2's os.Stat in TransitionGuard cause performance problems?

**Concern:** `TransitionGuard` is called by `DequeueNextQueuedItems` (a background sweep), `GetPendingGates` (every item-detail page load), and `TransitionBacklogItemStatus` (every user transition). Adding `os.Stat` to a hot path could cause slowdowns, especially if many items have a valid path.

**Answer:** The disk check is gated:
1. Only runs when `to == BacklogStatusInProgress` (not for all transitions).
2. Only when `item.PlanApproved && !item.SkipPlanning && item.PlanArtifactsPath != ""` (only for items that have a plan path).
3. The primary fix (Task 1) prevents new items from ever having a stale path, so this check will be a no-op for virtually all items going forward.

For the dequeue sweep, each candidate is already loaded from DB; adding one `os.Stat` per candidate is negligible compared to the existing DB lookups. The real concern is `GetPendingGates` being called on page load — but that already does `hasUnresolvedBlockers` DB lookups, so one more `os.Stat` is proportionate.

**Verdict:** Acceptable performance impact. The guard condition makes it a cold path for well-behaved items.

---

### Challenge 3: Does the plan address the "worktree cleaned up after successful triage" case?

**Concern:** Even with Task 1's fix, a successfully written `implementation/` directory can be deleted later by `CleanupTerminalItem` (when the item is done/archived and its work session worktree — which reuses the triage worktree directory — is cleaned up). If the item is then reopened, `PlanArtifactsPath` still points to the deleted directory.

**Answer:** Task 2 (TransitionGuard disk check) addresses this by blocking dequeue when the path is stale. The error surfaces to the user (and to the stuck-item detector) as a `CodeFailedPrecondition` on the next dequeue attempt. The user sees the item stuck in Queued with a clear error and can re-run triage to refresh the plan. This is the correct behavior: after worktree cleanup, the plan docs are gone and re-triage is actually the right recovery.

Task 1 addresses the "never written" case; Task 2 addresses the "was written, then deleted" case. Together they cover both scenarios from the research doc.

**Verdict:** Both scenarios are addressed. No gap.

---

### Challenge 4: Is there a race between os.Stat in TriggerTriage and the worktree directory creation?

**Concern:** The LLM is running in the worktree. Could there be a race where `os.Stat` runs before the LLM has flushed its writes?

**Answer:** No. `os.Stat` runs AFTER `triageCaller.CallBlocking(...)` returns (line 480). `CallBlocking` is synchronous — it only returns after the LLM process exits. By that point, all file writes are flushed (the claude process has exited). The only race would be if a filesystem sync is not guaranteed after process exit, which is not the case for local POSIX filesystems.

**Verdict:** No race condition.

---

### Challenge 5: Do existing tests need updating?

**Concern:** `TestTriggerTriage_should_AutoApprovePlan_When_AutoApprovePlanSet` and `TestTriggerTriage_should_NotAutoApprovePlan_When_AutoApprovePlanUnset` both assert `PlanArtifactsPath != ""`. After Task 1's fix, both tests still need to produce a real `implementation/` directory in the fake triage output for the assertion to hold. Do they?

**Answer:** Both tests use a fake headless caller that simulates the LLM output. They assert `PlanArtifactsPath != ""` today — which means their fake output MUST be creating the directory already (otherwise the tests would be asserting a currently-correct but soon-to-break behavior). Need to verify: if the fake LLM output in these tests does NOT create the directory, they'll start failing after Task 1. The implementation phase must inspect these two tests first and ensure their fake output creates `implementation/`.

**Verdict:** Low risk, but requires inspection during implementation. Noted as a pre-condition for Task 1.

---

## Validation Checklist

| Item | Status |
|------|--------|
| Fix preserves prior good PlanArtifactsPath on re-triage failure | ✓ Verified (nil pointer semantics) |
| os.Stat in TransitionGuard is performance-acceptable | ✓ Verified (cold path for well-behaved items) |
| Both "never written" and "deleted after use" scenarios addressed | ✓ Verified (Tasks 1+2 together) |
| No race condition in os.Stat timing | ✓ Verified (called after CallBlocking returns) |
| Existing auto-approve tests need inspection | ⚠ Needed before implementation |
| No schema changes required | ✓ (*string field, nil = no-op) |
| Error message improvement actionable (AC-7) | ✓ Task 3 covers it |

## Open Questions

1. Should a DB migration sweep clear existing stale `plan_artifacts_path` rows (paths that no longer exist on disk) as a one-time fix for affected items already in production? This is out of scope for this PR but worth a follow-up issue.

2. Should the stuck-item detector (`ReconcileStuck`) explicitly recognize the new `ErrPlanArtifactsMissing` error and surface a `PLAN_NOT_APPROVED`-like stuck reason? Currently it would be generic stuck behavior.
