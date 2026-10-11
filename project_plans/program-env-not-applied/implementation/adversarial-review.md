# Adversarial Review: program-env-not-applied

**Date**: 2026-09-24
**Verdict**: CONCERNS

Re-review scoped to the previous round's BLOCKED item. Verified directly against the current
`plan.md` text (not re-derived from scratch):

- **Original BLOCKER resolved.** Task 1.1.1d (`plan.md:202-224`) now exists and does exactly what
  was asked: temporarily reproduces pre-fix `initTmuxSession` behavior (checkout `instance_tmux.go`
  at `cdfd4e5cf2^` or an equivalent hand-edit stub matching the old `SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + i.UUID})`-only
  call), runs the new test, requires confirming `FAIL`, restores to HEAD, and requires confirming
  `PASS` again. Story 1.1.1's AC3 bullet (`plan.md:138-141`) was rewritten to require this red/green
  proof explicitly ("not merely observed to pass on HEAD today"), not just a HEAD-only pass. Epic
  1.1 also gained an explicit **Contingency** clause (`plan.md:108-111`) for the case where the
  red/green check doesn't reproduce a failure.
- **All four prior CONCERNS addressed:**
  - AC1 printenv-in-pane: Task 1.1.1c (`plan.md:185-191`) adds an explicit `tmux send-keys "printenv
    ANTHROPIC_BASE_URL" Enter` + `capture-pane -p` step asserting on the captured pane output, on
    top of the `show-environment` check.
  - AC5 root-cause mechanism: Task 1.1.1c's doc-comment instruction (`plan.md:192-199`) and Story
    1.1.1's AC bullet (`plan.md:142-147`) both now state the actual defect — pre-fix
    `initTmuxSession` called `SetExtraEnv` with only `STAPLER_SESSION_UUID`, no code path merged
    `ResolveProgramConfig(...).EnvVars` in — not just the fix commit SHA (the SHA is still cited,
    but as traceability, not as the root-cause statement itself).
  - Contingency/halt path: present twice — Epic 1.1's Contingency note (`plan.md:108-111`) and Task
    1.1.1d's own final bullet (`plan.md:218-221`, "If the test does **not** fail against the
    pre-fix stub, stop — do not proceed to Epic 1.2-1.4").
  - Epic 1.2 scope note: present at `plan.md:233-236`, acknowledging the epic broadens scope beyond
    requirements.md's literal ACs and explaining why it's included anyway. (The prior review's
    recommendation offered a choice between a requirements.md note or a plan.md note; the plan took
    the plan.md option, which satisfies the recommendation as given.)

No new BLOCKER was introduced by the repair edits. One new CONCERN was found in Task 1.1.1d's own
mechanics (see below) — the red/green check is sound in principle but under-specifies safety around
the production-code revert-and-restore step.

## Blockers

(none)

## Concerns

- [ ] **Task 1.1.1d's revert-and-restore of `session/instance_tmux.go` has no safety net if the
  step is interrupted mid-way.** The task (`plan.md:202-224`) offers "a local, uncommitted edit" as
  one of two equally-valid ways to reproduce the pre-fix stub, alongside a scratch worktree — but
  only the scratch-worktree path is naturally interruption-safe (the main tree is never touched). If
  the in-place-edit path is used and something interrupts the sequence between step 2 (revert) and
  step 4 (restore to HEAD) — the `go test` invocation panics, the implementing agent's session ends,
  a person gets pulled away — the repo is left with production code reverted to a pre-fix, broken
  state and no signal that this was intentional/temporary. That's the exact git-hygiene risk this
  repo's own global instructions warn about for destructive-adjacent git operations ("run `git
  status` first," never treat checkout/restore as risk-free). Nothing in the task tells the
  implementer to verify a clean starting `git status` before step 2, to prefer the scratch-worktree
  path as the default, or to check `git status` again after the final restore to confirm the tree is
  actually clean before moving on to Epic 1.2. — **Recommendation**: make the scratch worktree (or a
  tagged `git stash push -u -m <unique-tag>`, per this repo's shared-stash-stack convention) the
  required mechanism, not an alternative to an in-place edit; add a closing `git status --short`
  check as the task's last bullet, asserting no diff remains in `session/instance_tmux.go` before the
  task is considered done.

## Minors

- Task 1.1.1d's "Files" line (`plan.md:222-223`) lists only `server/services/session_service_create_test.go`,
  but the file actually being temporarily reverted and restored is `session/instance_tmux.go`
  (production code, in a different package). Worth listing both, given the git-hygiene concern above
  — a reader skimming "Files" for what this task touches would miss the one file that matters most
  for that risk.
- Carried over from the previous review round, still open: Epic 1.3 (`plan.md:370-401`) still
  narrows the program-ID-vs-command client-side check to the web Omnibar path without stating that
  the MCP session-creation path is moot (`server/mcp/tools_lifecycle.go`'s `program` param is a
  closed `mcpgo.Enum("claude", "aider")`, so it has no way to select a custom-registered program).
  Low cost to add one sentence; not required to close any AC.
