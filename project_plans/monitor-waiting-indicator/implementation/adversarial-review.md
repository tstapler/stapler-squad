# Adversarial Review: monitor-waiting-indicator

**Date**: 2026-09-09
**Verdict**: CONCERNS

## Blockers

None. The prior BLOCKER is resolved and re-verified: applied Task 1.1.1a's regex change and
Task 1.1.1b's summation generalization directly to `session/detection/binaries/claude.go` and
`pattern_set.go` in this worktree, then ran the real detector against AC3's current fixture
(`plan.md:196-204`) — `"...1 shell, 1 monitor still running"` followed by `"❯ "` then
`"  ? for shortcuts"`. Result: `StatusIdle`, count `0`, matching the plan's given/when/then
exactly. All edits were reverted after verification (`git status` clean — see below).

## Concerns

- [ ] **Tail-window residual risk is real and now honestly documented, but no follow-up ticket
  exists yet** — `plan.md`'s Known Limitations section (`plan.md:363-379`) is accurate: I
  independently reproduced its specific numeric claim, running the bare-`"❯ "`-only fixture (no
  `"  ? for shortcuts"` line) both pre-fix and post-fix. Pre-fix: `StatusWaitingForAgent`, count
  `1` (via the `monitors_still_running` fallback). Post-fix: `StatusWaitingForAgent`, count `2`.
  Exactly as claimed — the fix changes the count, not the underlying stuck-indicator exposure.
  The disposition ("accepted, pre-existing limitation... defer a general fix... to a future
  ticket") is reasoned, not hand-waved, but "defer to a future ticket" with no ticket filed is a
  disposition that's easy to lose track of once this plan ships. **Recommendation**: file the
  tracking ticket now (or note the ticket ID in the plan) rather than leaving the deferral as
  prose only.
- [ ] **Story-level Files roll-up omits two files touched by its own sub-tasks** —
  `plan.md:225-226` lists Story 1.1.1's Files as `binaries/claude.go`, `pattern_set.go`,
  `bug_regression_test.go`, but Task 1.1.1c also touches `pattern_set_test.go` (`plan.md:284`)
  and Task 1.1.1e touches `detector.go` (`plan.md:305`) — both real, both missing from the
  story-level summary. Cosmetic (task-level Files lines are individually correct), but a
  reviewer skimming only the story header would miss two touched files. **Recommendation**: add
  `pattern_set_test.go` and `detector.go` to the Story 1.1.1 Files line.

## Minors

- Both concerns from the prior review round are genuinely resolved, not just mentioned in
  passing: Task 1.1.1c (`plan.md:266-272`) now specifies a concrete zero-count-guard fallthrough
  case, and I independently verified the underlying behavior it describes — a synthetic
  `"✻ Churned for 52s · 0 monitors still running"` line, run through the post-fix detector,
  falls through the `WaitingForAgent` group entirely (resolves to `StatusSuccess` via
  `verb_duration_completion`) rather than returning `ok=true, count=0`. Task 1.1.1c
  (`plan.md:277-283`) also now specifies a concrete wrapped/split-line documented-limitation
  case, consistent with the Known Limitations entry (`plan.md:380-393`).
- Task/Epic numbering (1.1.1a–e, 1.2.1a–c) and the Dependency Visualization diagram
  (`plan.md:142-165`) are fully consistent with the actual task list — no gaps, no orphaned
  steps, no numbering collisions, despite the doc's growth to 402 lines.
- No merge-artifact residue found (no conflict markers, tables render cleanly, no dangling
  internal `plan.md:` self-citations that could go stale).
- Re-verification method: wrote a throwaway `session/detection/zzz_reverify_test.go`, applied
  the two production diffs from Task 1.1.1a/1.1.1b, ran `go test ./session/detection/... -run
  TestZZZ -v` and `go test ./session/detection/... -count=1` (full existing suite still green),
  then reverted both production files via `git checkout --` and deleted the throwaway test.
  `git status --short` at the end of this review shows only the pre-existing untracked
  `project_plans/monitor-waiting-indicator/` directory — no stray source-file edits left behind.
