# Research: Pitfalls — Autonomous Git Conflict Detection + Auto-Fix

Agent 4 (Pitfalls), SDD research phase for `backlog-pr-conflict-detection`.

## Scope

This document is grounded in the actual code paths this project extends:
`ReconcilePRPending` (`session/backlog_lifecycle.go:530-585`), `GetPRStatus`
(`session/git/worktree_git.go:338-438`), and `AutoReopenForPRFix`
(`server/services/backlog_service_triage.go:438-513`). One structural fact shapes
every pitfall below: **`AutoReopenForPRFix` does not itself run git commands.** It
transitions the backlog item, stuffs `fixContext` into the item's notes, and calls
`SpawnSessionFromItem(..., Autonomous: true)`. The actual rebase, conflict
resolution, and force-push will be performed by the spawned Claude Code agent's own
`Bash` tool calls inside its worktree, driven by whatever the fix prompt says. There
is currently **no Go-level guardrail** on what git commands that session runs — the
only control surface available to this project is (a) what the prompt tells it to
do/not do, and (b) what `ReconcilePRPending` does with the *result* on the next poll.
Every mitigation below has to work within that constraint.

---

## 1. `mergeable`/`mergeStateStatus` async computation — false positives and false negatives

GitHub computes `mergeable` and `mergeStateStatus` out-of-band after push/rebase
events; `gh pr view --json mergeable,mergeStateStatus` can return
`mergeable: UNKNOWN` / `mergeStateStatus: UNKNOWN` for a real, non-trivial window
(seconds to low tens of seconds, longer under GitHub-side load) before settling to
`MERGEABLE`/`CONFLICTING`. This is a documented GitHub API behavior, not a bug —
the field is backed by an async merge-commit computation job, not a synchronous
git operation.

**False positive risk (treating transient `UNKNOWN` as a conflict):**
`ReconcilePRPending` polls on a fixed interval and is stateless across polls (no
memory of the previous cycle's value). If `UNKNOWN` is treated as "not
mergeable → conflict", every PR that was *just* pushed to (including a normal,
successful human push, or the very push the conflict-fix session itself makes)
would transiently read as conflicting immediately afterward. Concretely:

- The fix session's own force-push at the end of a successful rebase re-triggers
  GitHub's async mergeable computation. If the very next poll cycle lands inside
  that computation window, `ReconcilePRPending` would see `UNKNOWN`, misclassify
  it as still-conflicting, and spawn *another* fix session on top of a PR that
  just got fixed — burning an iteration of `maxAutoReworkIterations` for no
  reason and potentially racing the two sessions against the same branch (see
  §4/§5).
- This is a machine-timescale problem: the poll interval and GitHub's computation
  window are the same order of magnitude, so this isn't a rare edge case, it's a
  near-guaranteed occurrence on the poll cycle immediately following any push to
  a `pr_pending` item.

**False negative risk (missing a real conflict because of a cached/stale `UNKNOWN`):**
The inverse failure is treating `UNKNOWN` as "assume healthy, skip this cycle" with
no re-check forcing mechanism. If `GetPRStatus` is ever memoized/cached at any layer
(it isn't today, but this is exactly the shape of bug the `getHeadCommitSHA` incident
warns about — see §5), a genuinely `CONFLICTING` PR that was read as `UNKNOWN` on
one cycle could get "skip and retry next cycle" applied indefinitely if some future
change adds backoff/caching keyed on "no actionable signal this cycle" without
re-validating on the next one. Today's poll loop is a plain re-query every cycle
with no state carried forward, which actually protects against this — but it means
the *only* correct place to fix the false-negative risk is to guarantee `UNKNOWN`
never causes an early-return/short-circuit that skips a subsequent real check.

**Recommended handling** (for Phase 3 to formalize): treat `UNKNOWN` as
"skip this cycle, re-poll next cycle" — never as `CONFLICTING`. Do not spawn a fix
session on `UNKNOWN` alone. Only require the transition CONFLICTING for at least
one full poll cycle. Because `ReconcilePRPending` already re-queries fresh from `gh`
every cycle with no persisted "last known conflict state," this degrades gracefully:
worst case is one extra poll interval of latency before a real conflict is acted on,
which is acceptable given there's no SLO on this path (per requirements.md,
"Performance SLO: not specified"). Do **not** add a "require 2 consecutive
CONFLICTING reads" debounce that persists state across polls — that's new state to
get wrong, and the existing poll-and-recheck loop already gives resilience for free
as long as `UNKNOWN` never counts as anything actionable in either direction.

---

## 2. `.gitignore` corruption interaction

Today's session independently found and manually fixed a real recurring bug: backlog
worktree-spawned sessions have corrupted `.gitignore` (gutted from ~100 lines to a
placeholder) on 3 separate branches (PRs #147, #148, and #150's predecessor — per
requirements.md's Problem Statement). The root cause of *that* corruption is not this
project's scope, but this project's fix-session **will operate on exactly the
branches most likely to already carry it**, for two reasons:

1. Branches that sat in `pr_pending` long enough to drift into merge conflict are
   disproportionately the same branches that have been open long enough / touched
   by enough autonomous rework cycles for the pre-existing `.gitignore` bug to have
   had a chance to strike (`maxAutoReworkIterations` review-rework cycles run in the
   same worktree-spawn code path implicated in the `.gitignore` incidents).
2. A rebase is exactly the git operation most likely to *surface* a latent
   `.gitignore` conflict as a visible merge conflict (if `main` also touched
   `.gitignore`, or if the corrupted version differs enough from `main`'s version to
   conflict on rebase) — and also the operation most likely to make an *already*
   silently-corrupted `.gitignore` look like a legitimate "resolve this hunk" target
   to an agent with no other signal that the file was wrong before the conflict even
   started.

**Concrete risk:** the fix session, told generically "rebase and resolve conflicts,"
has no way to distinguish "this `.gitignore` conflict is a normal rebase-forward" from
"this `.gitignore` was already silently corrupted before the rebase started, and now
I'm about to pick a resolution that further ratifies (or re-derives) the corrupted
version." An LLM resolving a conflict marker in a mangled ~5-line `.gitignore` has no
context that the file used to be ~100 lines — it will confidently "resolve" the
conflict using only the two versions git shows it, neither of which is necessarily
correct.

**Defensive checks worth carrying into Phase 3 planning** (this project's own scope
is detection + reuse of the existing fix path, but the fix-session prompt is
explicitly in scope per requirements.md's "conflict-specific fixCtx/prompt addition"):

- **Line-count / size sanity check on `.gitignore` specifically**, before vs. after
  the fix session's rebase, as a cheap post-hoc guard `ReconcilePRPending` (or a
  follow-up reconciliation step) could run: if `.gitignore` shrank by some large
  factor (e.g. >50%) across the fix session's commits, flag rather than silently
  accept — this is a narrow, file-specific version of a broader "did this fix
  session's diff look like a real fix or a regression" check, and is directly
  actionable because the exact corruption pattern (gutted to a placeholder) is
  already known from today's incidents.
- **Prompt-level guidance**: since `AutoReopenForPRFix` already lets this project
  customize `fixCtx` (per Scope: "conflict-specific fixCtx/prompt addition"), the
  conflict-fix prompt should explicitly instruct the agent to treat unexpectedly
  short/placeholder-like config files (`.gitignore` named specifically, given the
  known incident pattern) as suspicious and prefer taking the longer/more complete
  side of a conflict over guessing, rather than mechanically picking "ours" or
  "theirs" or synthesizing a fresh version.
- **Scoping what files the fix session is allowed to touch** was considered but is
  not cleanly achievable within this project's constraints: `AutoReopenForPRFix`
  spawns a full generic work session (same path as CI/review fixes), and file-level
  tool restriction is not a capability that path currently exposes — building it
  would cross into "dedicated rebase-only flow," which is explicitly out of scope
  (requirements.md, Out of Scope: "A dedicated, narrower rebase-only flow ... 
  explicitly rejected"). This is a real gap between what would be ideal and what
  this project's constraints allow; Phase 3 should flag it explicitly as an accepted
  risk rather than silently rely on prompt guidance alone.

---

## 3. Force-push risk and blast radius

A rebase necessarily rewrites the branch's commit SHAs, so landing it on the PR's
remote branch requires a force-push. This is a materially different operation than
a normal fix commit + push (the CI-failure and review-comment fix paths today
presumably just add commits and push normally — worth Phase 3 confirming, since
`GetPRStatus`/`AutoReopenForPRFix` code has no push logic at all; it's entirely
inside the spawned agent's own tool use).

**Is there a meaningful safety difference between an interactive AI assistant asking
before a force-push (this repo's own global CLAUDE.md-equivalent convention) and a
fully autonomous backend pipeline force-pushing with no human in the loop?**
Functionally, no — the blast radius of an errant force-push (silently discarding
commits a human or another process pushed to the same branch in the interim) is
identical regardless of who/what triggered it. What differs is *who's available to
say no*: an interactive turn has a human in the loop who can veto in real time; this
pipeline's only checkpoint is whatever validation happens automatically before the
push, since by definition nobody is watching a `pr_pending` reconciliation cycle
fire at 3am. That makes the autonomous case *higher* risk per-incident (no real-time
veto), even though this project's `maxAutoReworkIterations` cap bounds how many times
it can happen per item.

**Mitigations, evaluated against this project's actual code surface:**

- **`--force-with-lease` instead of `--force`**: meaningfully reduces blast radius
  at near-zero cost, and is enforceable — this is exactly the kind of instruction
  that belongs in the conflict-fix `fixCtx` prompt (in scope per this project's
  "conflict-specific fixCtx/prompt addition"). `--force-with-lease` refuses the
  push if the remote branch's tip has moved since the fix session last fetched it
  (e.g., a human pushed a manual fix to the same PR while the autonomous session
  was working), converting a silent data-loss push into a visible failure the fix
  session (or the next poll cycle) has to handle. This is the single highest-value,
  lowest-cost mitigation available within this project's scope and should be a
  hard requirement in the conflict-fix prompt, not just a suggestion.
- **Always working in an isolated worktree first**: already true structurally —
  `SpawnSessionFromItem` (per the session-creation-registry conventions and this
  codebase's existing worktree-per-session model) gives every spawned session its
  own worktree. This means a bad rebase-in-progress never corrupts the shared
  working tree other sessions read from, but it does **not** protect the shared
  `.git` object database or shared refs — see §5, this is exactly the class of gap
  the `getHeadCommitSHA` incident exposed.
- **Requiring the rebase to preserve all original commits' net diff before allowing
  the push**: attractive in principle (a cheap `git diff <old-tip>..<new-tip>` after
  rebase, gated on it being empty or "only whitespace/mechanical," would catch a
  rebase that silently dropped a hunk while resolving conflicts) but this project
  has no code-level hook to gate the push on — the push happens inside the spawned
  agent's own tool use, not through a Go function this project controls. The only
  way to get this check today is, again, prompt guidance ("verify `git diff` between
  your pre-rebase and post-rebase branch tip against the base doesn't drop any net
  change before pushing") or a post-hoc reconciliation check on the *next*
  `ReconcilePRPending` cycle (e.g., re-diffing the PR against its pre-fix base and
  flagging suspiciously small diffs) rather than a hard pre-push gate. Phase 3
  should decide whether "post-hoc detection + flag for manual review" is an
  acceptable substitute for "pre-push hard gate," since the latter isn't achievable
  without building push-time interception this project's scope explicitly excludes
  (Out of Scope: "Automatic force-push / merge without going through the same
  worktree+session flow every other autonomous fix uses" — read narrowly, this
  says don't bypass the worktree+session flow, not that a gate can't exist within
  it, but no such gate exists in the current spawn path to hook into).

---

## 4. Retry/loop risk — the fix session's own commits triggering new failures

`maxAutoReworkIterations = 3` (`server/services/backlog_service_triage.go:37`) caps
*how many* work sessions get spawned per item, shared identically for the existing
review-rework path (`workCount` counts all `session.SessionRoleWork` sessions
regardless of what triggered them — see `AutoReopenForPRFix`,
lines 452-465). It does **not** evaluate whether each successive attempt is
converging or diverging. A bad rebase resolution in attempt 1 that introduces a
subtle break (wrong side of a conflict picked, a hunk silently dropped per §3) could:

- Pass its own CI cleanly (a `.gitignore` corruption, for instance, is exactly the
  kind of change that doesn't fail CI — it's semantically silent) while leaving the
  PR in a worse state than before, consuming an iteration for a "fix" that was a
  regression.
- Or, worse, produce a rebase that itself conflicts again on the *next* cycle (e.g.
  because the force-push landed a tree that diverges further from `main`, or because
  a second concurrent push to `main` happened during the fix session's work),
  re-triggering `ReconcilePRPending`'s conflict detection and spawning attempt 2
  against an already-degraded base.

Today's cap counts attempts, not outcomes — it has no signal to distinguish
"third attempt, steadily converging" from "third attempt, each one worse than the
last." Given the `.gitignore` corruption precedent (a real regression that produces
no CI signal), a purely attempt-counted cap can exhaust all 3 iterations while
making the PR strictly worse each time, then leave it in `pr_pending` for manual
action anyway — arguably no worse than doing nothing, but with 3x the git history
churn and 3x the force-push blast-radius exposure along the way.

**Signal Phase 3 should evaluate for "getting worse, stop" detection**, ranked by
how cheaply they fit this project's existing signals:

1. **Diff-size / file-touched trend across iterations**: `ReconcilePRPending` (or
   the spawn path) could record, per iteration, which files changed and roughly how
   much. If iteration N's fix touches files unrelated to what iteration N-1's
   conflict was about (scope creep symptomatic of "guessing"), or if the same file
   keeps getting touched by every iteration without the conflict resolving, that's
   a cheap, mechanical divergence signal — no semantic understanding required.
2. **Re-conflict on the very next poll cycle**: if the newly-pushed branch is
   `CONFLICTING` again within one poll interval of the fix session completing, that
   is strong evidence the "fix" didn't actually fix anything (either it never
   resolved the original conflict, or its own push against a since-moved `main`
   immediately created a new one). This is directly observable with the new
   `mergeable`/`mergeStateStatus` signal this project adds — worth flagging to
   Phase 3 as a natural complement to the plain iteration cap, since the data
   needed (conflict state before and after each spawn) already exists once this
   project's core detection ships.
3. **`.gitignore`-specific size-regression check from §2** is a special case of
   (1) narrow enough to implement cheaply and directly tied to a known real
   incident pattern, so it's worth calling out on its own even though it's subsumed
   by the general diff-trend idea.

None of these are in this project's Medium-appetite scope to *implement* — they're
research findings to hand to Phase 3, which already flagged the cap-sharing question
as an open Rabbit Hole. The key finding for Phase 3: **the existing cap is a safety
net against infinite retries, not against monotonically worsening ones**, and the
conflict-detection signal this project adds (mergeable state before/after a fix
attempt) is the cheapest available building block for closing that gap later.

---

## 5. Concurrency — shared parent-repo git races

This session already found and fixed a real, production-observed instance of this
exact risk class: `session/git/util.go`'s `getHeadCommitSHA`
(fixed across two commits, `dce6a644` then refined in `4cbb5294`,
"fix(git): keep go-git as the fast path; validate + retry before falling back to
CLI"). The root cause: go-git's `repo.Head()` does an unlocked, direct read of the
ref file, which raced a **concurrent `git worktree add` against the same parent
repo** and returned a syntactically-valid 40-hex-char SHA that corresponded to no
real object in the repository at all (absent from `git cat-file -t`,
`git rev-list --all`, `git reflog show --all`, and `git fsck --unreachable`) — not
merely stale, genuinely nonexistent. This poisoned `Worktree.base_commit_sha` and
caused a false `UNVERIFIABLE`/"(no diff available)" review verdict on a real,
correct fix. The fix (still the current state of the code) keeps go-git as the fast
path but validates every read against go-git's own object store and retries before
falling back to the git-CLI (`git rev-parse HEAD`), which is immune to the race
because it relies on git's atomic-rename ref-update guarantees rather than
unlocked direct file reads.

**Does this project's rebase operation face the same class of risk?** Yes, and
arguably a higher-stakes version of it, for two reasons:

1. **No existing serialization**: there is no lock (mutex, singleflight, or
   otherwise) in `session/git/*.go` that serializes git operations against a shared
   parent repository across worktrees. `worktree.go`'s `singleflight.Group` (used
   for `IsDirtyWithHint`) is scoped per-`GitWorktree`-instance / per-key, not a
   cross-process or cross-worktree lock on the shared `.git` directory — it
   coalesces *duplicate concurrent calls for the same worktree*, it does not
   protect one worktree's git operation from another worktree's concurrent git
   operation against the shared object database. A rebase running in the
   conflict-fix session's worktree, and a separate backlog work session's commit /
   `git worktree add` / GC happening concurrently in a sibling worktree of the same
   parent repo, are exactly the scenario that produced the corrupted-SHA incident —
   except now instead of a read racing a write, it's a **rebase's own object writes
   and ref updates** (which touch the shared `.git/objects` and, depending on git's
   internals, potentially trigger auto-gc) racing another worktree's concurrent
   writes.
2. **Rebase is a heavier, longer-running git operation than the single
   `HEAD` read that broke last time.** A rebase performs many sequential commits,
   each writing new objects and moving refs, over a longer wall-clock window than
   a single `git rev-parse`. That's a proportionally larger window for a
   concurrent worktree operation elsewhere in the same parent repo to interleave
   with it. If go-git is used anywhere in the *rebase's own implementation path*
   (this project's rebase will actually be done by the spawned agent shelling out
   to real `git rebase` via its Bash tool, not go-git — so the specific go-git
   ref-file race from the incident doesn't directly apply to the rebase mechanics
   themselves), the higher-order risk is git-level: **concurrent `git` processes
   (real git CLI, not go-git) operating on the same repository's shared
   `.git/objects` and `.git/refs` from different worktrees**.

**What's actually safe to run concurrently against the same repo from multiple
worktrees, per git's own documented concurrency model:**

- **Safe**: read-only operations in different worktrees that don't touch shared
  refs (`git status`, `git diff` against already-resolved SHAs, `git log`) — object
  reads are content-addressed and immutable, so no torn-read risk once an object
  exists. `git worktree add` itself is documented as safe to run concurrently in
  modern git (it uses lock files under `.git/worktrees/<name>/locked`), though the
  incident above shows the *consumer* (go-git reading HEAD immediately after)
  wasn't safe even when the producer was.
- **Risky/documented-unsafe**: operations that write into the **shared** object
  database or shared refs without per-worktree isolation — this includes `git gc`/
  `git repack` (git's own docs warn these can race concurrent operations in other
  worktrees without `--force`/proper locking), and any operation that updates a ref
  also tracked/read elsewhere (branch refs are per-worktree via `.git/worktrees/*/HEAD`
  but tags and the shared object database are not). A `git rebase` in one worktree
  writes new commit/tree/blob objects into the *shared* `.git/objects` — this is
  git's designed-for-concurrency case (content-addressed writes, no read-modify-write
  race on objects themselves) and is generally safe even when interleaved with
  other worktrees' object writes. The actually fragile part, per the incident
  already fixed, is **downstream tooling (go-git, or anything else) reading refs/
  HEAD immediately after a git-CLI worktree operation without git's own
  atomic-rename read guarantees** — i.e. the risk isn't rebase-vs-rebase git-level
  corruption (git's object model is designed to make that safe), it's **this
  codebase's own non-git-CLI code paths (go-git reads) observing a torn/incomplete
  state mid-operation**, exactly as already diagnosed and fixed for
  `getHeadCommitSHA`.

**Concrete recommendation for Phase 3**: audit every other `session/git/*.go`
function that uses go-git (not just `getHeadCommitSHA`, which is already fixed) for
the same unlocked-read-during-concurrent-worktree-write pattern, since a rebase
happening in one worktree is now a new, longer-duration source of exactly the kind
of concurrent parent-repo activity that triggered the original incident. This
project's own new code (`GetPRStatus`'s extended `gh pr view` call) is unaffected —
it's a pure `gh` CLI/network call with no local git state read — but the
*reconciliation loop calling it* runs `g.IsPRMerged` and `g := git.NewGitWorktreeFromStorage(...)`
per item on every poll (`session/backlog_lifecycle.go:544`), meaning any go-git call
inside that `GitWorktree` construction or its methods is now running concurrently,
on a timer, against however many `pr_pending` items exist, at the same time other
backlog work sessions (including this project's own spawned fix sessions) are doing
`git worktree add`/rebase/push against the same parent repos. Worth a targeted
`grep -rn "git.PlainOpen\|repo.Head()\|go-git" session/` sweep in Phase 3 planning
to confirm `getHeadCommitSHA` was the only affected call site, not an assumption.

---

## Summary of Actionable Findings for Phase 3

| # | Risk | Recommended handling within this project's scope |
|---|---|---|
| 1 | `UNKNOWN` mergeable state race | Never treat `UNKNOWN` as conflicting; rely on the existing stateless re-poll loop rather than adding new debounce state |
| 2 | `.gitignore` (or similar) corruption re-surfacing via rebase | Add explicit prompt guidance in the conflict-fix `fixCtx` to prefer the more-complete side of a conflict on suspiciously-short config files; flag file-scoping as an accepted gap given the "reuse `AutoReopenForPRFix`" constraint |
| 3 | Force-push blast radius | Hard-require `--force-with-lease` (not `--force`) in the conflict-fix prompt — cheapest, highest-value mitigation available given no push-time code hook exists |
| 4 | Worsening retry loop | Existing cap counts attempts, not outcomes; hand Phase 3 the mergeable-state-before/after-each-attempt signal as the cheapest available "getting worse" detector, now producible once this project ships |
| 5 | Concurrent worktree git races | Rebase itself is git-safe (content-addressed writes); the actual risk is this codebase's own go-git call sites reading torn state — audit `session/git/*.go` beyond the already-fixed `getHeadCommitSHA` in Phase 3 |
