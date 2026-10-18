# Research: fix-triage-artifact-path

> Describes the code *before* #1072; the line references below are to that earlier state.

## Confirmed Root Cause

**File:** `server/services/backlog_service_trigger_triage.go` lines 655–688

For SDD-mode items, `TriggerTriage` computes:
```go
pap = filepath.Join(triageWorkDir, "project_plans", sanitizedTitle, "implementation")
```
and unconditionally places it in the update struct:
```go
update := session.BacklogItemUpdate{
    PlanArtifactsPath: &pap,   // <-- no os.Stat guard; always written to DB
    PlanApproved:      &approvalReset,
    ...
}
```

The `os.Stat(pap)` check at line 676 only gates auto-approval — it does NOT prevent the empty/missing path from being persisted. When the LLM writes `project_plans/<name>/` but not `project_plans/<name>/implementation/`, or when the worktree is later deleted, the DB retains a stale path.

## Data Model

- `BacklogItemUpdate.PlanArtifactsPath` is `*string` (`session/repository.go:646`).  
- Passing `nil` skips the DB update entirely — the repo layer checks `if update.PlanArtifactsPath != nil` (`session/ent_repository_backlog.go:1226`).  
- This means **Option A** (guard persistence on `os.Stat` success, leave `nil` on failure) is implementable without any schema change.

## ApprovePlan (secondary issue)

`server/services/backlog_service_lifecycle.go:1022-1028`:
1. If `PlanArtifactsPath == ""` → `"no plan artifacts found — run TriggerTriage first"`.
2. If `os.Stat(PlanArtifactsPath)` fails → `"plan artifacts path %q does not exist on disk — re-run TriggerTriage"`.

Both return `CodeFailedPrecondition`. Neither error distinguishes "never created" from "deleted after creation." The message "re-run TriggerTriage" is misleading when the real answer is "triage ran but produced no `implementation/` directory."

## TransitionGuard — No Disk Check

`session/domain/backlog.go:663-665`:
```go
if item.PlanApproved && !item.SkipPlanning && item.PlanArtifactsPath == "" {
    return ErrPlanArtifactsRequired
}
```
Only checks empty string — never calls `os.Stat`. A stale (non-existent) non-empty path passes this guard today. After the primary fix (don't persist stale path), this guard remains correct for new runs, but existing stale rows in the DB are still a concern.

## UI Derive Logic

`web-app/src/lib/backlog/itemActions.ts:137-138`:
```ts
const isGatedOnPlanApproval = !item.skipPlanning && !item.planApproved;
const hasPlan = !!item.planArtifactsPath;
```
`hasPlan = true` whenever `planArtifactsPath` is a non-empty string — no disk check. So a stale path in the DB shows the "Approve Plan" button, which then fails `ApprovePlan`'s `os.Stat`.

## Existing Test Coverage

Covering `PlanArtifactsPath` in SDD mode:
- `TestTriggerTriage_should_AutoApprovePlan_When_AutoApprovePlanSet` — asserts `PlanArtifactsPath != ""`
- `TestTriggerTriage_should_NotAutoApprovePlan_When_AutoApprovePlanUnset` — asserts `PlanArtifactsPath != ""` and `PlanApproved == false`

**Gap:** No test covers the case where the `implementation/` directory was never created and verifies that `PlanArtifactsPath` stays empty in the DB.

## Fix Options

| Option | Description | Risk |
|--------|-------------|------|
| A (preferred) | Guard `PlanArtifactsPath` persistence on `os.Stat` success. Leave it `nil` in update when dir missing. | Low — mirrors existing auto-approve guard, no schema change. |
| B | Always set `pap`, but overwrite with `""` on `os.Stat` failure. | Slightly worse — clears a previously-valid path from a prior successful triage run when re-triage fails. |
| C | Add disk check to `TransitionGuard` for the `Queued → InProgress` case. | Adds `os.Stat` to a hot path; also doesn't fix the stale path written to DB. |
| D | Add migration to clear `plan_artifacts_path` for rows where the path no longer exists. | Data repair only; doesn't prevent recurrence. Useful as a supplement. |

**Recommendation:** Option A as the primary fix, with an improved error message in `ApprovePlan` (AC-7) and a `TransitionGuard` disk check (AC-4) as defense in depth.

## Worktree Lifecycle

The triage worktree at `worktrees/triage-<itemID>_<suffix>/` is:
- Kept (committed, branch renamed) on successful triage
- Reused as the work session worktree (same directory) via `backlogWorkBranchSlug`
- Cleaned up by `CleanupTerminalItem` → `cleanupItemWorktreesExcept` when a `SessionRoleWork` session is cleaned up on terminal transitions (done/archived)

After cleanup, `PlanArtifactsPath` still points at the deleted directory. This is the "was written, now gone" scenario. The primary fix (Option A) does not address this; it addresses "was never written." For the deleted-after-use case, `TransitionGuard`'s disk check (AC-4) provides a safety net.
