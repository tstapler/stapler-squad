# Requirements: fix-triage-artifact-path

## Item
**Title:** Investigate false "triage artifact does not exist" error  
**Item ID:** 1cfae441-8953-41e6-803d-959062119d4f  
**Category:** bugfix

---

## Problem Statement

When an SDD-mode backlog item is in the **Queued** state (plan approved, waiting for a work session slot), users see a red error box:

> plan artifacts path '/home/tstapler/.stapler-squad/workspaces/.../worktrees/triage-<itemID>_.../project_plans/<name>/implementation' does not exist on disk — re-run TriggerTriage

The item's structural check for `Queued → In Progress` also shows ✗ with description "no unresolved blockers, and plan approved (or skipped) with artifacts recorded." A visible "Approve Plan" button is displayed. Clicking it triggers the error above.

The triage run **appeared** to complete successfully — the item reached Queued status and the `PlanArtifactsPath` field is set to a non-empty string. However, the path it references never exists (or no longer exists) on disk.

---

## Root Cause (confirmed via code archaeology)

**File:** `server/services/backlog_service_trigger_triage.go`, lines 655–688

In `TriggerTriage` for SDD-mode items, `PlanArtifactsPath` (`pap`) is computed as:
```go
pap = filepath.Join(triageWorkDir, "project_plans", sanitizedTitle, "implementation")
```

This path is saved to the database **unconditionally** — regardless of whether the LLM actually created the `implementation/` subdirectory. The auto-approval check at line 676 correctly guards on `os.Stat(pap)`, but on failure it only warns and skips approval; it does **not** clear `PlanArtifactsPath`.

Result: the item is left with a non-empty `PlanArtifactsPath` pointing to a directory that was never created (or was later deleted). The UI derives `hasPlan = !!planArtifactsPath` and shows "Approve Plan"; `ApprovePlan` then fails `os.Stat` and returns the confusing error.

**Two sub-scenarios** produce this state:
1. The LLM wrote `project_plans/<name>/` but not the `implementation/` subdirectory (e.g. the SDD triage completed but structured its output differently).
2. The triage worktree was deleted after the path was saved (disk cleanup, `CleanupTerminalItem` running after a prior reopen cycle, etc.).

---

## Acceptance Criteria

| # | Criterion | Testable |
|---|-----------|----------|
| AC-1 | After a successful SDD triage run, `PlanArtifactsPath` is only persisted when the `implementation/` subdirectory actually exists on disk (`os.Stat` succeeds). | Yes — unit test TriggerTriage with SDD mode where `implementation/` is absent. |
| AC-2 | When the `implementation/` directory does not exist after triage, `PlanArtifactsPath` is left empty (not set to the non-existent path), so the UI shows `retry_triage` instead of `approve_plan`. | Yes — verify DB field stays empty. |
| AC-3 | `ApprovePlan` returns a clear, actionable error when `PlanArtifactsPath` is set but the path no longer exists on disk (the "path exists in DB but was deleted" scenario). Error message should differentiate between "never written" and "was written, now gone" where inferable. | Yes — unit test ApprovePlan with path-not-on-disk precondition. |
| AC-4 | The `TransitionGuard` check for `Queued → InProgress` rejects items whose `PlanArtifactsPath` is non-empty but the directory does not exist on disk — prevents items from being dequeued into an unresolvable state. | Yes — unit test TransitionGuard with a path set to a non-existent directory. |
| AC-5 | Existing items already in the DB with a non-empty `PlanArtifactsPath` that no longer exists on disk are surface-able (a migration or startup sweep logs them as warnings). | Yes — manual verification / startup log check. |
| AC-6 | No regression: items whose `implementation/` directory IS correctly created by the LLM continue to auto-approve and flow through Queued → InProgress without interruption. | Yes — existing full-lifecycle SDD test still passes. |
| AC-7 | The error message in `ApprovePlan` no longer says "re-run TriggerTriage" when the path was never written; instead it says something like "triage did not produce plan artifacts — re-run triage" (or equivalent). | Yes — inspect error message text in unit test. |
