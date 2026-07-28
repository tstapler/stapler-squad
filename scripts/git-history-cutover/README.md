# Git History Cutover — reclaiming the ~15GB of committed GIFs/benchmarks

## Status quo (as of 2026-07-16)

`.git` is ~17GB. `git lfs` tracking was added in commit `c9935823` ("chore(repo):
enable Git LFS for demo GIFs and benchmark baselines") — that stops the bleeding
**going forward**: new commits to `docs/demos/*.gif` and `benchmarks/**/*.{txt,json}`
are stored as LFS pointers, not raw blobs. It does **not** shrink the ~15.7GB of
GIFs + ~2.5GB of benchmark dumps already sitting in history across thousands of
prior commits.

This directory holds the tooling for the follow-up step: rewriting history so
those old blobs are replaced with LFS pointers everywhere, reclaiming the space.
**None of these scripts run automatically and none of them touch this working
repo directly.** They operate on a disposable mirror clone until the final,
explicit push step.

## Why this is not a "just run it" operation

- Rewriting history changes every commit hash from the first offending commit
  onward. The rewritten `main` is a different set of commits than the current
  one — landing it requires a force-push to both `origin` and `personal`
  (**both public**), which is a hard violation of "never force push" unless
  explicitly overridden for this one operation.
- This repo currently has 90+ active worktrees (`git worktree list`), each
  checked out against the *current* commit hashes. After a force-push, every
  one of those branches still has valid history locally (nothing is deleted
  from anyone's disk automatically) but is now based on a `main` that no
  longer exists on the remote — anyone who pulls or bases new work on the old
  `main` will diverge further. Every worktree with commits ahead of `main`
  needs to be replayed onto the new history (see step 3).
- LFS pointer migration means anyone with an existing local clone needs LFS
  installed and configured before pulling the rewritten history, or checkouts
  of `docs/demos/*.gif` / `benchmarks/**` will show pointer text instead of
  real content.

## The runbook, when you're ready

### 0. Prerequisites
- `git-lfs` and `git-filter-repo` installed (`brew install git-lfs git-filter-repo`
  — both already present on this machine).
- Push access confirmed to both `origin` and `personal`.
- Everyone with in-flight work in a worktree/branch has been told a rewrite is
  imminent (or the operation is done at a genuinely quiet point).

### 1. Analyze (read-only, safe to run anytime)
```bash
./01-analyze.sh
```
Reports current `.git` size, the largest historical blobs, and an estimate of
post-migration size. Re-run this right before step 2 to confirm the picture
hasn't changed.

### 2. Rewrite history in a scratch mirror (does not touch this repo)
```bash
./02-rewrite.sh --i-understand-this-rewrites-history
```
Without the confirmation flag, this only prints the commands it *would* run.
With it:
- Clones `origin` as a fresh `--mirror` into `/tmp/stapler-squad-cutover-<timestamp>/`.
- Runs `git lfs migrate import --everything --include="docs/demos/*.gif,benchmarks/**/*.txt,benchmarks/**/*.json"`
  against that mirror — this rewrites every commit reachable from every ref in
  the mirror, replacing matching blobs with LFS pointers, and writes a
  commit-hash mapping to `<mirror>/.git/lfs-migrate-map.txt` (old-sha new-sha
  pairs, one per line — produced via `git lfs migrate`'s own log; if using
  filter-repo instead, the mapping is `<mirror>/.git/filter-repo/commit-map`).
- Prints the new `.git` size and leaves the mirror in place for inspection —
  it does **not** push anything.

### 3. Replay each active worktree's unique commits onto the new history
For every worktree/branch that has commits not yet in the *old* main:
```bash
./03-cutover-branch.sh --mirror /tmp/stapler-squad-cutover-<timestamp> --branch <branch-name>
```
This finds `old-main..<branch-name>`'s unique commits, looks up their merge-base
in the commit map, and rebases the branch onto the equivalent new commit in the
mirror. If the merge-base itself isn't in the map (branch forked from something
outside the rewritten ref set), it falls back to cherry-picking the unique
commits directly onto new-main, and will stop for manual conflict resolution
if any commit touches a migrated path in a way that doesn't apply cleanly.

Run this for every branch in `git worktree list` with unmerged work before
proceeding to step 4, or those commits are at risk of being orphaned once the
old `main` ref is gone from the remote.

### 4. Push the rewritten history and notify
```bash
./04-push-and-notify.sh --mirror /tmp/stapler-squad-cutover-<timestamp> --i-understand-this-force-pushes
```
Force-pushes the mirror's rewritten `main` (and any replayed branches from
step 3) to both remotes (auto-detects `origin`/`upstream-fanatics`/`personal`/
`upstream` — whichever exist; remote naming has drifted across machines for
this repo, see 05-local-resync.sh's comment). Prints a message pointing
anyone with an existing local clone/worktree at step 5 below.

### 5. Each affected machine/clone runs the resync script
```bash
./05-local-resync.sh --i-understand-this-resets-local-history
```
Run this on every OTHER machine/clone with this repo (not one of the 90+
worktrees sharing this machine's object database — see below). Without the
confirmation flag it only reports what it found. With it:
- Tags current `HEAD` and stashes any uncommitted changes (by explicit
  pathspec, never a blanket `-u`) under a timestamped backup name first —
  nothing is ever silently discarded.
- Fetches the remote and either fast-forwards (if nothing was rewritten out
  from under this branch) or hard-resets to the new history.
- Prints the backup tag/stash name so you can recover anything afterward, and
  how to discard the backup once you've confirmed nothing was lost.

For one of THIS machine's 90+ active worktrees: if it has no commits unique
to it (nothing ahead of the old `main`), just delete it and create a fresh
one — there's nothing to resync. If it has real in-progress commits, use
step 3's `03-cutover-branch.sh` against the mirror instead of this script,
before the old base disappears from the remote.

## What these scripts deliberately do NOT do

- They don't touch this working checkout's `.git` — everything destructive
  happens in a disposable `/tmp` mirror until the explicit push in step 4.
- They don't auto-discover and replay all 90+ worktrees for you — step 3 is
  one branch at a time, on purpose, so each replay can be checked before
  moving to the next.
- They don't delete the mirror clone afterward — keep it until you're
  confident the cutover succeeded, in case a branch needs re-replaying.
