# BUG-112: Two Different "Empty Diff" Review Guards Produce Misleading FAIL/UNVERIFIABLE Verdicts When A Feature Is Already Fully Committed By A Prior Session Incarnation [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-09-14, investigating backlog item `1c08da73` (durable guidance-request
mechanism), which cycled through 20+ work-session incarnations over "review" without ever
getting a real reviewer verdict on already-complete, already-tested code.

**Impact**: A backlog item whose acceptance-criteria work is genuinely complete and committed
can get stuck indefinitely bouncing between `in_progress` and `review`, because both review
entry points compute their diff **relative to the session that requests review**, not relative
to `origin/main`. When a prior session incarnation already committed and merged all the work
before a fresh session (a "resume", or a new work-session spawned onto the same worktree)
calls `request_review`, that fresh session's own base-commit-to-HEAD diff is empty even though
`git diff origin/main..HEAD` is substantial.

## Two distinct degrade paths, same root condition

1. **`ReviewGateRunner.Run`** (`session/review_gate.go:326`, reached via `request_review` →
   `TriggerReviewForSession`) — `committedDiffEmpty` guard fires immediately: hard FAIL, "no
   committed changes were found for this session," no headless call attempted at all.
2. **`TriggerReReview`** (`server/services/backlog_service_triage.go:2803`, reached when a
   stuck/no-active-session item is reconciled, e.g. `AutoRespawnReview`) — an empty diff routes
   to the "codebase-read" path instead, which requires
   `headless.DefaultCapabilitySelfCheck.Ensure` to pass before granting the reviewer
   Read/Grep/Glob tool access. If that self-check is failing (for whatever reason — see below),
   the item gets the generic **UNVERIFIABLE: codebase-read capability self-check failed**
   verdict instead of FAIL, which is what item `1c08da73` saw twice in a row.

Both guards exist for a legitimate reason (BUG-047/BUG-065: don't silently PASS/UNVERIFIABLE a
review that has nothing real to look at) — but neither one distinguishes "no work happened at
all" from "the work happened, just not within this particular session incarnation's own base
commit range." Confirmed live via
`~/.stapler-squad/workspaces/<ws>/logs/staplersquad.log`: item `1c08da73`'s branch had a real,
substantial diff vs `origin/main` (59 files, 6254 insertions) at the exact moment path #1 fired
its "no committed changes were found for this session" FAIL — the work was not missing, just
invisible to that session's own `wt.BaseCommitSHA`.

## Why this reads as a capability_check.go bug (and isn't)

Because path #2's failure message names `capability_check.go` explicitly
("...claude CLI/config does not appear to grant WorkDir+AllowedTools+PermissionMode read
access..."), it strongly suggests a bug in that file. Three prior work-session attempts on this
item changed `capability_check.go`/`capability_check_test.go` (a real, separately-justified
nil-pool-caching fix and a test-compile fix) without ever resolving the recurring UNVERIFIABLE
verdict, because the actual trigger — an empty per-session diff reaching the codebase-read path
at all — was never addressed. The fix that actually worked was committing outstanding work so
a *fresh* review request would see a non-empty diff — but even that only fully resolves the
loop when the review is requested from an **active session with a fresh, non-empty base-to-HEAD
diff**, not when reconciliation later re-triggers `TriggerReReview` against the same
already-fully-committed worktree.

## Reproduction

1. Complete all AC work in a work session, commit and merge cleanly. End the session without
   calling `request_review` (or let it end/resume such that a *new* `ItemSession` row with a
   fresh `BaseCommitSHA` gets created before `request_review` is called).
2. Resume/spawn a fresh session onto the same worktree; call `request_review` with zero new
   commits of your own.
3. Observe: `ReviewGateRunner.Run` FAILs with "no committed changes were found for this
   session," even though `git diff origin/main..HEAD` in that same worktree is non-empty.
4. Alternatively, let the item go stuck long enough for reconciliation to call
   `TriggerReReview`: observe UNVERIFIABLE via the codebase-read/capability-self-check path
   instead, for the identical underlying reason.

## Suggested fix direction (not implemented here — out of scope for the item this was found on)

`committedDiffEmpty` (path #1) and the `workSessionDiff == ""` branch (path #2) should
distinguish "no commits anywhere on this branch since it diverged from `origin/main`" (the
real no-op case both guards are meant to catch) from "no commits *by this particular session*,
but the branch itself has real, uncommitted-nowhere-else work relative to `origin/main`" (this
bug's case) — e.g. by falling back to a `origin/main`-relative diff when the per-session
base-to-HEAD diff is empty, rather than treating per-session emptiness as equivalent to
no-op-ness. This needs its own scoped `sdd:fix-bug` pass; it touches two separate review entry
points shared by every backlog item, not just this one.

## Related

- BUG-045 (fixed): a different bug in the same "codebase-read" empty-diff path
  (`resolveCodebaseWorkDir` falling back to the shared main checkout) — same general area of
  the review harness, different specific defect.
- BUG-047/BUG-065 (referenced in `session/review_gate.go`): the original motivation for
  blocking on an empty diff rather than silently PASS/UNVERIFIABLE-ing a no-op session — this
  bug is about that guard's false-positive rate, not a case against having it.
