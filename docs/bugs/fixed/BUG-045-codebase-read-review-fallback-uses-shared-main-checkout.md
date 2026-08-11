# BUG-045: Codebase-Read Review Fallback Grants the Reviewer Live Filesystem Access to the Shared Main Checkout — Not the Item's Own State — When the Item's Worktree Is Gone [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-24)
**Discovered**: 2026-07-24, investigating a stale/wrong FAIL verdict on item `693c2700` while manually reopening it for revision
**Fixed**: 2026-07-24 — `server/services/backlog_service_triage.go`
**Impact**: When a headless review's primary diff computation comes back empty (`getWorkSessionDiff` finds nothing) and the item's dedicated worktree has already been cleaned up, `resolveCodebaseWorkDir` (`server/services/backlog_service_triage.go:2382` at time of filing) fell back to `repoPath` and the reviewer was granted live Read/Grep/Glob filesystem access to that directory as "the codebase to review." For every backlog item in this project, `repoPath` resolves to the same shared path — the single main checkout the human operator (and any concurrent Claude Code session) actively works in day to day, with `git stash`/merges/deploys constantly changing its working-tree state. Whatever unrelated, uncommitted work happens to be sitting there at the exact moment a fallback review runs got handed to the reviewer as if it were the item's own codebase — producing a plausible-sounding but completely wrong FAIL verdict.

**Confirmed live**: item `693c2700`'s work session did the real, correct work (item ID display, copy/deep-link, board-pane restore), shipped a clean PR #216 with 22 CI checks green — but its review session (which hit this fallback because the item's worktree had been reaped) reported FAIL, describing "a tmux orphaned-client fix (BUG-042) and log-stream debug gating changes" — content matching an entirely unrelated, unrelated piece of uncommitted work that happened to be sitting in the shared main checkout's working tree at that moment, not anything on the item's actual branch.

## Root Cause (confirmed)

`resolveCodebaseWorkDir`:
```go
func (s *BacklogService) resolveCodebaseWorkDir(ctx context.Context, repoPath string, workSession *session.ItemSessionSummary) (dir string, exists bool) {
	dir = repoPath
	if workSession != nil {
		if wt, wtErr := s.storage.GetWorktreeDataBySessionUUID(ctx, workSession.SessionUUID); wtErr == nil && wt.WorktreePath != "" {
			dir = wt.WorktreePath
		}
	}
	info, statErr := os.Stat(dir)
	return dir, statErr == nil && info.IsDir()
}
```

**Important nuance found during investigation** (not visible from reading the bug doc's original description alone): when a worktree *row* is present in storage but its on-disk directory has since been deleted, this function was already safe — `dir` gets overwritten to `wt.WorktreePath`, and `os.Stat` on that (now-missing) path correctly returns `exists=false`. That specific shape was already covered by an existing regression test (`TestTriggerReReview_should_BlockInsteadOfFalseFail_When_WorktreeGoneAndDiffUnrecoverable`, added by a prior worktree-hardening pass) and remained correct throughout this fix.

The actual, uncaught gap is one level up: when `workSession != nil` but its worktree data **cannot be resolved at all** — `GetWorktreeDataBySessionUUID` returns `wtErr == nil, wt.WorktreePath == ""` (the ent query's `ent.IsNotFound` branch returns a zero-value `GitWorktreeData` with **no error**, both when the underlying session row was itself hard-deleted — a real code path, see `Storage.DeleteInstance`, which explicitly deletes the session's `Worktree` row via a manual cascade before deleting the session row itself — and, structurally identically, when a session legitimately never had a dedicated worktree at all) — `dir` in the original code stayed `repoPath` from initialization, and since `repoPath` (the shared main checkout) obviously exists on disk, `exists` came back `true`. The caller's own pre-existing guard (`if workSessionDiff == "" && !codebaseWorkDirExists { ...block... }`) never fired, because `codebaseWorkDirExists` was `true` — even though the directory being reported as "the item's own state" had no actual relationship to this item's work.

This is the exact trap `getWorkSessionDiff` (same file) is already careful about for the diff-computation path: it uses `GetGitDiffRef` with an *explicit branch ref* precisely so the diff reads from the git object store (branch tip vs. base SHA) rather than the fallback directory's live working-tree/HEAD state. `resolveCodebaseWorkDir`'s codebase-read fallback had no equivalent protection — Read/Grep/Glob tools operate on the literal filesystem, which has no notion of "diff against a ref."

## Fix Applied

Implemented as the minimal, well-scoped `sdd:fix-bug` version the bug doc's own "Recommended Routing" section called for — refuse the fallback with an inconclusive state, mirroring `ReviewGateRunner.Run`'s (`session/review_gate.go`) established refusal to hand the reviewer a diff it could not positively compute. The throwaway-worktree-materialization option (suggested fix #2) was not needed: no caller or test in this codebase relies on genuinely reviewing a directory-mode session's `repoPath` as its own dedicated state when a work session with an unresolvable worktree exists, so refusing outright closes the gap without narrowing any real, exercised capability.

`resolveCodebaseWorkDir` now distinguishes three cases instead of two:

1. **No work session at all** (`workSession == nil`) — `repoPath` is the only directory available and there is nothing item-specific it could be masking; falls back to `repoPath` exactly as before (this is the case every existing "empty-diff codebase-read succeeds" test in this codebase actually exercises — e.g. `TestTriggerReReview_EmptyDiff_UsesWorkDirAndCodebaseAccessPrompt` — none of them attach a work session at all).
2. **A work session exists but its worktree cannot be resolved** (storage lookup errors, or resolves to an empty `WorktreePath`) — **refuses** the fallback outright: returns `exists=false` (with `dir=repoPath`, kept only for the caller's existing log message). This is the new BUG-045 guard.
3. **A work session exists with a recorded worktree path** — stats that path directly (never `repoPath`) — this branch's behavior is unchanged from before the fix; it was already correct.

```go
func (s *BacklogService) resolveCodebaseWorkDir(ctx context.Context, repoPath string, workSession *session.ItemSessionSummary) (dir string, exists bool) {
	if workSession == nil {
		info, statErr := os.Stat(repoPath)
		return repoPath, statErr == nil && info.IsDir()
	}
	wt, wtErr := s.storage.GetWorktreeDataBySessionUUID(ctx, workSession.SessionUUID)
	if wtErr != nil || wt.WorktreePath == "" {
		return repoPath, false
	}
	info, statErr := os.Stat(wt.WorktreePath)
	return wt.WorktreePath, statErr == nil && info.IsDir()
}
```

The caller (`TriggerReReview`, same file) needed no changes — its existing `if workSessionDiff == "" && !codebaseWorkDirExists { ...block, RecordDegradedReviewVerdict, notify... }` guard already implements exactly the refusal behavior the bug doc asked for; it was just never reached because `resolveCodebaseWorkDir` was reporting the wrong `exists` value for this case.

## Files Affected

- `server/services/backlog_service_triage.go` — `resolveCodebaseWorkDir` rewritten to refuse the `repoPath` fallback when a work session exists but its worktree cannot be resolved
- `server/services/backlog_service_triage_test.go`:
  - Two new unit tests directly on `resolveCodebaseWorkDir`: refuses when a work session's worktree is unresolvable; still allows the fallback when there is no work session at all
  - One new end-to-end regression test reproducing item `693c2700`'s exact live shape via `TriggerReReview`
  - One existing test (`TestAutoRespawnReview_DeadWorkSession_TombstonedThenRespawns`) updated: its fixture's "dead" work session never had an Instance/Worktree row recorded, which is structurally the same "no real per-item worktree" shape this fix now refuses — its assertions were updated from "the headless pool must be called" to "the pool must NOT be called against `repoPath`, and an explicit UNVERIFIABLE verdict must be recorded instead," matching the corrected, safe behavior. The test's actual point (tombstoning a dead session unblocks `AutoRespawnReview` rather than leaving it stuck behind `hasActiveWorkSession`) is preserved and still verified — only the previously-unsafe codebase-read expectation changed.

## Verification

- **`TestResolveCodebaseWorkDir_should_RefuseFallback_When_WorkSessionWorktreeUnresolvable`** — a work session exists with a `SessionUUID` that has no corresponding Instance/Worktree row in storage at all (simulating the row itself being reaped). Asserts `resolveCodebaseWorkDir` returns `exists=false`, even though `repoPath` is a real, existing directory.
- **`TestResolveCodebaseWorkDir_should_AllowFallback_When_NoWorkSessionAtAll`** — companion guard for the case that must *not* regress: `workSession == nil` still returns `exists=true` against a real `repoPath`.
- **`TestTriggerReReview_should_BlockInsteadOfReviewingSharedCheckout_When_WorktreeRowReaped`** — full end-to-end reproduction of the live incident: a real git repo standing in for the shared main checkout, seeded with a file (`unrelated-in-progress-work.txt`) representing an operator's unrelated uncommitted work; a work session whose worktree row was never created (reaped); a headless pool scripted to return a FAIL verdict naming that file — which would prove the bug if it were ever actually invoked. Asserts `pool.callCount() == 0` (the pool is never spent reading the wrong directory), the recorded verdict is `UNVERIFIABLE` (not the false FAIL the live incident produced), and an operator notification fires.
- **`TestAutoRespawnReview_DeadWorkSession_TombstonedThenRespawns`** (updated) — re-verified after the fix: `AutoRespawnReview` still succeeds and tombstones the dead session (`EndedAt` set), but the pool is no longer called and an `UNVERIFIABLE` verdict is recorded instead of the (pre-fix) real headless-pool response.
- **All three new/changed tests verified to fail against pre-fix code**: stashed the fix in `backlog_service_triage.go` (keeping the new tests), reran — `TestResolveCodebaseWorkDir_should_RefuseFallback_When_WorkSessionWorktreeUnresolvable` failed (`exists` was `true`), and `TestTriggerReReview_should_BlockInsteadOfReviewingSharedCheckout_When_WorktreeRowReaped` failed exactly as the live incident did: the pool *was* called (`callCount()==1`) and the recorded verdict was the scripted false `FAIL` naming `unrelated-in-progress-work.txt`, not `UNVERIFIABLE`. Restored the fix; all pass again.
- Full `go test ./server/services/...` — all passing (63s), no regressions beyond the one intentionally-updated test above.
- Full `go test ./session/...` — all passing, unaffected (this fix only touches `server/services`).
- `golangci-lint run ./server/services/... ./session/...` — 0 issues.
- `gofmt -l` on both changed files — no output (clean).

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Semantic/Intent gap. The function's doc comment already named the correct invariant ("mirroring `ReviewGateRunner.Run`'s refusal to hand the reviewer a diff it could not positively compute") but the implementation didn't actually deliver it for one of the two ways a worktree can be unavailable — it correctly handled "row present, directory gone" but silently mishandled "row itself unresolvable," conflating "the fallback directory exists on disk" (a filesystem fact about `repoPath`, which is always true) with "the fallback directory reflects this item's own state" (the actual invariant the caller needs). A boolean named `exists` was doing double duty for two different questions.

**Earliest achievable enforcement**: The regression tests are the practical ceiling here, same as the two prior bugs in this exact function's history (BUG-040's original worktree-row-outlives-directory finding, and this one) — "does the DB-recorded state for a session still correspond to real, isolated evidence" is fundamentally a runtime storage/filesystem condition, not something a type system or lint rule can express. The type-level improvement made: `resolveCodebaseWorkDir`'s three cases (no session / session-but-unresolvable / session-with-real-path) are now written as three explicit, sequential returns rather than one mutable `dir` variable threaded through a single `os.Stat` call at the end — this doesn't add compiler-enforced safety, but it does mean a future reader/editor of this function can no longer accidentally reintroduce the "unconditionally fall through to `repoPath`" default the bug hinged on, since there is no longer a shared fallthrough path for the two structurally different "no per-session worktree" reasons to both land in.

**Recurring shape**: This is the **same underlying function's second bug** in this codebase's history — `resolveCodebaseWorkDir`'s own doc comment already documents one prior incident (PR #173, "Backlog History feature Broken," the DB-row-outlives-directory case) that a previous fix addressed by adding the `os.Stat` existence check this bug doc's root cause discusses. That fix was real and correct, but incomplete: it hardened the "directory literally deleted out from under a real recorded worktree" path while leaving the adjacent "worktree can't be resolved at all" path using the old, unguarded fallback. Worth naming for future review of this function or its neighbors: **"a hardening fix that closes the specific failure mode it was written against, while leaving a structurally adjacent failure mode in the same function unexamined."** This is a narrower instance of BUG-044's already-named "reactive machinery mistaken for a guarantee" pattern — here the guarantee (`exists` means "safe to review") was correct for the specific incident that motivated writing the check, but the check's own logic didn't generalize to cover every way the precondition it was meant to enforce could fail.

## Related

- The bug doc's own root-cause section cross-references the confirmed-live incident on PR #173 ("Backlog History feature Broken") that motivated the pre-existing `os.Stat` existence check this fix builds on — that prior fix remains correct and unchanged by this one.
- `693c2700` itself does not need re-fixing for this bug — its real PR (#216) is already correct and ready to merge; only its *displayed* status/verdict was wrong, and that's the downstream symptom this fix prevents going forward, not something to repair retroactively.
