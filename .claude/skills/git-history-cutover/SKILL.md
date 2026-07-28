---
name: git-history-cutover
description: Drive the stapler-squad Git LFS history-rewrite cutover — reclaiming storage from bloated LFS history (docs/demos/*.gif, benchmarks/**/*.{txt,json}) via a disposable mirror clone, per-branch replay for the 90+ active worktrees, coordinated force-push to both remotes, and guided resync for every other affected clone. Use when the LFS storage quota is exceeded and blocking CI, when asked to shrink .git size from historical LFS bloat, or when picking up an in-progress cutover across sessions.
---

# Git History Cutover — LFS Storage Reclamation

**Goal**: Take `.git` from bloated-with-old-LFS-history to reclaimed, with every
affected worktree/clone back on the new history and nothing silently lost.

Scripts live in `scripts/git-history-cutover/` (`01-analyze.sh` through
`05-local-resync.sh`) — full runbook and rationale in that directory's
`README.md`. This skill is the guided, decision-aware driver on top of them:
it tells you which script to run when, tracks the per-worktree replay work,
and holds the line on the safety gates none of the scripts skip on their own.

---

## Read this first: rewriting history does NOT immediately free the quota

GitHub's own docs are explicit: LFS objects removed from history stay in
remote storage and keep counting against the quota until they're truly
unreferenced by any ref — then GitHub garbage-collects them automatically,
but only after roughly a 30-day grace period. There is no supported
self-service "delete now" button; the only way to force it sooner is a
support ticket, or deleting and recreating the repo (not viable here — this
is a live, actively-developed repo with two remotes, issues, and PR history).

**Implication**: this operation stops the bleeding and reclaims space over
the following weeks. It does not unblock CI today. If CI needs to be
unblocked immediately, that's a separate, smaller fix (e.g. skip the LFS
checkout step for jobs that don't actually need `docs/demos/*.gif` — check
with the user before doing this, since it's a CI-workflow change with its
own review surface).

---

## Before touching anything: current shape of the problem

Re-verify these numbers before running step 1 — they drift:

```bash
git lfs ls-files -s | wc -l              # current-tip LFS file count
git lfs ls-files --all | wc -l           # LFS objects across ALL history
du -sh .git/lfs/objects                  # actual bytes on disk
git worktree list | wc -l                # how many worktrees will be affected
```

As of the last pass: 213 current files (260 MB), 428 objects across history
(**54 GB** on disk), ~90 active worktrees sharing this machine's object
database. All of it is `docs/demos/*.gif` — the `benchmarks/**/*.{txt,json}`
`.gitattributes` rule currently matches nothing.

---

## The runbook

### 0. Prerequisites and the freeze point

- `git-lfs` and `git-filter-repo` installed.
- Push access confirmed to **both** remotes (`git remote -v` — naming has
  drifted across machines for this repo; expect `origin` +
  `upstream-fanatics`, or `origin` + `personal`, or similar — don't assume).
- **Pause or drain active work first.** Check `git worktree list` for
  worktrees with real in-progress commits (branches that aren't
  `worktree-agent-*` scratch names, or that show unpushed commits ahead of
  `main`). A rewrite mid-flight orphans any of them still being actively
  edited. This is not a "run it anytime" operation — do it at a genuinely
  quiet point, or explicitly pause the sessions/agents that own those
  worktrees first.

### 1. Analyze (read-only, safe to run anytime)
```bash
./scripts/git-history-cutover/01-analyze.sh
```
Confirms current size, largest historical blobs, and which active worktrees
have commits not yet on `main` — the list you'll need for step 3.

### 2. Rewrite in a disposable mirror (never touches this working repo)
```bash
./scripts/git-history-cutover/02-rewrite.sh --i-understand-this-rewrites-history
```
Clones `origin` as a fresh `--mirror` into `/tmp/stapler-squad-cutover-<ts>/`
and rewrites every ref in that mirror only. Decide here whether you're
converting the GIFs to LFS pointers or deleting them from history outright —
the script as written does an LFS migrate; adapt the command inside it (or
use `git filter-repo --path docs/demos --invert-paths` instead) if full
deletion is what's actually wanted this time. Confirm with the user which
one before running — they have different tradeoffs (migrate keeps the
content just un-billed; delete reclaims the most space but the demos are
gone from history for good).

### 3. Replay every worktree with unique commits
For each branch step 1 flagged as having commits not on the old `main`:
```bash
./scripts/git-history-cutover/03-cutover-branch.sh --mirror=<path> --branch=<name>
```
Run this once per branch, checking the output before moving to the next —
it does not batch across all worktrees automatically, on purpose. **Use
TaskCreate to track this list explicitly** (one task per branch needing
replay) so a long list doesn't lose an entry mid-session, especially if this
step spans a context compaction or a fresh session.

### 4. Push and notify
```bash
./scripts/git-history-cutover/04-push-and-notify.sh --mirror=<path> --i-understand-this-force-pushes
```
Force-pushes the mirror to every canonical remote it detects. This is the
hard-to-reverse step — confirm with the user immediately before running it,
even if they already approved the overall operation; a force-push to two
public remotes is exactly the kind of action that warrants a final explicit
go-ahead in the moment, not just standing approval from earlier in the
conversation.

### 5. Resync every other affected clone/machine
Point anyone else at:
```bash
scripts/git-history-cutover/05-local-resync.sh --i-understand-this-resets-local-history
```
It backs up their state first (tag + explicit-pathspec stash) and either
fast-forwards or hard-resets depending on whether their branch was actually
rewritten. For this machine's own worktrees: delete-and-recreate the ones
with no unique commits (nothing to resync); anything with real commits
should already have been replayed in step 3.

---

## Standing safety gates this skill does not relax

- Never skip the freeze-point check in step 0 because "it'll probably be
  fine" — the worktree count makes silent collateral damage the default
  outcome of rushing this.
- Steps 2 and 4 both require their explicit `--i-understand-this-*` flag —
  never add that flag on the user's behalf without them confirming in the
  same turn you're about to run it.
- If `git filter-repo` isn't installed and the user wants to proceed anyway
  with `git lfs migrate` alone, that's a real behavior difference (migrate
  only handles LFS pointer conversion, not arbitrary path deletion) — say so
  explicitly rather than silently substituting one for the other.

## Related

- `scripts/git-history-cutover/README.md` — full script-level detail and
  rationale for each step's design.
- `/sync-remotes` — the other operation that force-pushes to both of this
  repo's remotes; read its "Known Conflict Patterns" section for what a
  concurrent fork/upstream sync looks like if one is in flight at the same
  time as a cutover (don't run both simultaneously).
- `/git-worktrees` — general worktree isolation guidance, useful background
  for step 3's replay mechanics.
