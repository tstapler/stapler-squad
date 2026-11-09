---
name: worktree-disk-cleanup
description: Use when stapler-squad's git worktrees (session worktrees under ~/.stapler-squad/workspaces/*/worktrees, Claude Code's own .claude/worktrees/agent-*, and any ad hoc worktree elsewhere registered to this repo) are consuming significant disk space and need triage/removal. Triggers on "clean up worktrees", "orphaned worktrees", "disk space" + "worktree".
---

# Worktree Disk Cleanup

Bulk-classify every `git worktree` registered against the repo, remove the
high-confidence ones automatically, and route the ambiguous ones to cheap
per-worktree review agents instead of guessing.

## Why this needs a process, not ad hoc deletion

This repo accumulates worktrees from three sources: stapler-squad sessions
(`~/.stapler-squad/workspaces/*/worktrees/`), Claude Code's own worktree
isolation (`.claude/worktrees/agent-*`), and manual ones a person creates.
A single sweep (2026-09-11) found 148 worktrees eating 178GB, most from
abandoned experiments — but naive heuristics (e.g. "no PR = delete") are
wrong often enough that blind automation would lose real unshipped work.
GitHub PR state is the only ground truth strong enough to auto-delete on.

## Step 0 — Exclude backlog triage worktrees first, before any other check

`server/services/backlog_service_trigger_triage.go`'s `TriggerTriage` creates
a dedicated git worktree per headless SDD triage run (branch name
`<prefix>triage-<item-id>`, path matching `*/worktrees/triage-<item-id>*`)
purely at the git level — it has no `Instance` row, no `Worktree` ent row, and
is therefore invisible to every one of stapler-squad's own automated sweeps
(`session/worktree_consistency_sweep.go`, `SessionRetentionSweeper`,
`HibernationSweeper`). **This skill is the only thing in the whole system
that can see it, which makes it the only thing that can protect it.**

"No PR ever opened" is the *expected, healthy* state for one of these, not a
staleness signal: triage deliberately stops at committing
`project_plans/<name>/implementation/plan.md` and leaves the backlog item
unapproved until a human reviews the plan — a PR, and the work-session
worktree that would carry it, don't exist yet by design, sometimes for
months. Likely root cause (2026-10-17 investigation, inferred — no sweep
logs survive from the relevant window to directly confirm it): this skill's
own Step 3 guidance used to list "SDD planning artifacts" as an example of
a *trivial* diff, which is backwards — that commit is the entire
deliverable of the triage run, not boilerplate. Applying this skill (or the
same heuristic, pre-dating the skill's 2026-09-15 write-up — see the
2026-09-11 sweep noted above) to a worktree shaped like this would correctly
observe "no PR, clean, old," then have a review agent see only a plan.md
diff and call it trivial — matching four backlog items whose triage
worktrees were found deleted (directory gone, branch/commits intact, since
this skill never deletes branches) after sitting ~2 months idle with an
unapproved plan. Every automated sweep in the Go codebase was independently
ruled out (none of them can even see a triage worktree — see above), making
this skill the only mechanism left that explains the symptom.

Before running Step 1's inventory, or classifying any worktree in Step 2:
- Flag any worktree whose branch matches `triage-<uuid>` (with or without a
  configured branch prefix) or whose path contains `/worktrees/triage-`.
- If the stapler-squad backlog API/MCP tools are reachable, look up the
  matching backlog item (by item-id suffix of the branch/path) and check
  its `plan_approved`/status. Route to **Keep** unconditionally if the plan
  is unapproved or the item is still `idea`/`queued`/`ready`.
- If the API isn't reachable, still route to **Keep** by default — the
  presence of `project_plans/<name>/implementation/` content with no PR is
  itself the signal, not an input to a triviality judgment. Only fall
  through to Step 2's normal classification once a human confirms the
  linked item's plan was approved and superseded (e.g. the work session
  already shipped a PR off a *different* worktree).

## Step 1 — Inventory

```bash
git worktree list --porcelain > worktrees.porcelain   # path/HEAD/branch per entry
gh pr list --repo <owner>/<repo> --state all --json number,state,headRefName,mergedAt,updatedAt --limit 1000 > prs.json
```

Parse the porcelain output into `{path, branch, head}` records (skip the
`bare`/main entry). Cross-reference each branch against `prs.json` by
`headRefName`. Exclude every worktree Step 0 flagged as Keep before doing
anything else with the remaining list.

## Step 1b — Liveness gate (before classifying anything)

```bash
.claude/skills/worktree-disk-cleanup/worktree-liveness.sh <path>   # one line: VERDICT path reasons
```

Run it on every worktree. It checks (a) any process whose cwd is inside the tree
(Claude Code, shells, dev servers), (b) stapler-squad sessions in every state DB
(`~/.stapler-squad/sessions.db`, `instances/*`, `workspaces/*`) whose `worktrees.worktree_path`,
`existing_worktree` or `path` match, and (c) a Claude transcript or HEAD reflog touched in the
last `RECENT_HOURS` (default 2).

| Verdict | Meaning | Action |
|---|---|---|
| `LIVE` | process running, or Active/Creating/Hibernated/Restoring session points at it | Keep. Never remove, never snapshot-and-remove |
| `CONFIGURED` | non-archived Paused/Stopped session still references it | Keep unless the user approves; removal also strands that session's worktree row |
| `RECENT` | transcript or reflog activity within the window | Keep this round |
| `IDLE` | no signal | Continue to Step 2 |

Cloud/remote Claude sessions run in their own checkouts and never use local worktrees, so
they need no check. Run the script from a context where your own shell isn't inside the target
(it will report your own process as `LIVE`).

## Step 2 — Classify each worktree

For each entry, compute:
- `git status --porcelain` in the worktree dir → dirty or clean
- `git rev-list --count main..<branch>` → commits ahead of main
- `git log -1 --format=%cI <branch>` → last activity date
- PR states for that branch from `prs.json`

Then bucket:

| Bucket | Condition | Action |
|---|---|---|
| **Safe** | PR state MERGED or CLOSED, and any dirty files are just build junk (see below) | Auto-remove |
| **Stale, no PR** | No PR ever opened, clean tree, `ahead > 0`, untouched >10 days | Needs individual review (Step 3) |
| **Manual review** | Dirty tree with *real* (non-junk) uncommitted diffs, regardless of PR state | Never auto-remove — list for the user |
| **Keep** | Open PR, or no-PR but active in the last 10 days, or detached HEAD | Leave alone |

**Build-junk patterns** to ignore when deciding if a dirty tree is "really"
dirty (these regenerate on every build and carry no unshipped work):
`\.ent-gen\.stamp`, `\.proto-gen\.stamp`, `\.tmux-build\.stamp`,
`\.asdf-install\.stamp`, `^\?\? bin/`, `^\?\? gen/`, `^\?\? coverage\.(html|out)`,
`^\?\? stapler-squad$` (the built binary), `^M  \.gitignore`, `go\.sum$`,
`web-app/src/gen/`, `tsconfig\.tsbuildinfo`, `session/ent/` (generated ent
code, gitignored per repo `CLAUDE.md`), `third_party/tmux`,
`tests/e2e/test-results/`, `docs/registry/`. A dirty tree is "real" only if
lines remain after stripping these.

**Important exception**: a MERGED-PR worktree can still have *new* real
uncommitted work sitting on top (someone kept using the worktree after the
PR shipped) — check the commit/dirty-diff dates, not just the PR state. If
the diff touches actual source files and the activity is recent, route it to
manual review even though the PR merged.

## Step 3 — Individual review for the "stale, no PR" bucket

These are the ambiguous ones: real commits, no PR, old. Don't bulk-delete
them on a heuristic — dispatch one cheap agent (`model: haiku`) per worktree
in parallel to make an individual call. Give each agent:

- The worktree path and branch name
- Instruction to run, from the main repo checkout: `git log main..<branch>
  --oneline`, then `git show <sha> --stat` on each unique commit
- Instruction to check whether the work already landed another way —
  `git log --all --grep='<commit message>'` for duplicate/superseded
  content (batch worktree-creation tooling sometimes re-does the same
  planning/setup commit across many branches under different SHAs), or
  whether the diff is genuinely trivial boilerplate (a single
  auto-generated scaffold file, a CI experiment with no real content).
  **SDD planning artifacts (`project_plans/**/plan.md` and siblings) are
  never trivial** — they are triage's actual deliverable; a worktree whose
  only diff is one of these should already have been excluded in Step 0,
  so seeing one here at all means Step 0's check was skipped or failed, not
  that the content is disposable
- Instruction to check `git status --porcelain` in the worktree itself for
  uncommitted work
- A hard requirement to end the reply with exactly one line:
  `DECISION: DELETE - <reason>` or `DECISION: KEEP - <reason>`, response
  under ~80 words

For a `(detached)` HEAD entry (no branch name), have the agent use
`git -C <path> log -5 --oneline` and `git merge-base --is-ancestor <sha> main`
instead of `main..<branch>`.

Launch all of these in one message (parallel `Agent` calls) so they run
concurrently — they're read-only investigations, cheap on haiku, and
independent of each other.

## Step 3b — Triage the "Manual review" bucket (dirty with real diffs)

Snapshot first (Step 4's `worktree-salvage.sh`, which is non-destructive), then dispatch one
read-only agent per snapshot (`model: haiku`, parallel) with the export dir
(`~/.stapler-squad/salvage/<name>-<ts>/`: `meta.txt`, `uncommitted.patch`, `commits.patch`).
Each agent decides whether the work is still worth keeping by checking:
- does `git apply --check --reverse uncommitted.patch` succeed on current `main` (already landed)?
- do the touched files/functions still exist, and does `git log main --since=<snapshot base date> -- <files>`
  show the same area rewritten or the feature shipped another way (grep for the new symbols/tests)?
- for `commits.patch`: `git cherry main <branch>` and `git log --all --grep` for equivalents.
- is it anything other than throwaway (scratch notes, debug prints, a half-done approach superseded
  by a merged PR)? SDD `project_plans/**` content is never throwaway.

Each ends with exactly one line: `VERDICT: SUPERSEDED - <evidence>`, `VERDICT: KEEP - <what's
unique>` or `VERDICT: UNSURE - <question for the user>`. Present the user a table (worktree,
verdict, one-line evidence, export dir) to review; `SUPERSEDED` ones are removal candidates, all
others stay. Sanity-check each `SUPERSEDED` claim by opening the cited commit/PR yourself.

## Step 4 — Snapshot, then remove

Every worktree with real uncommitted work or commits not on a remote/merged branch is snapshotted
before removal:

```bash
out=$(.claude/skills/worktree-disk-cleanup/worktree-salvage.sh <path> [base-ref]) || { echo "KEEP: snapshot failed"; continue; }
```

This writes `refs/salvage/<name>/<ts>` (in the shared object DB; survives worktree removal;
restore with `git worktree add <path> <ref>`) and a review tree at
`~/.stapler-squad/salvage/<name>-<ts>/` (`uncommitted.patch`, `commits.patch`, `stat.txt`,
`meta.txt`). It drops build junk and files over 5MB (listed in `meta.txt`), and exits nonzero
without verifying a non-empty patch. **A nonzero exit means the worktree is kept.** Exit 2 =
nothing to salvage (clean), fine to remove. Purge salvage refs/dirs older than 60 days only
after the user agrees.

```bash
git worktree remove --force <path>   # --force needed: junk/dirty files block a plain remove
git worktree prune -v                # clean up stale worktree admin metadata afterward
```

Never touch the `manual_review` or `keep` buckets, or any `LIVE`/`CONFIGURED`/`RECENT` worktree
from Step 1b, without the user's
explicit go-ahead — that bucket is where real unshipped work lives. Get
confirmation on removal *scope* (which buckets) before running any
`git worktree remove`, even when every entry in a bucket looks safe — this
is a bulk destructive operation over other people's (or your own past
sessions') work.

## Step 5 — Prune orphaned shared node_modules directories

`web-app/`'s `node_modules` is a symlink into
`~/.stapler-squad/shared-node-modules/<hash>/node_modules` (see
`scripts/shared-node-modules.sh` and
`docs/explanation/package-management-evaluation.md`) rather than a full
per-worktree install — one directory per distinct `bun.lock` (+ `bunfig.toml`) +
local `file:`-dependency content, shared by every worktree whose
`web-app/` currently hashes to it. Removing a worktree in Step 4 does not
clean these up; run this step after Step 4 so a hash's last referencing
worktree is already gone before judging it orphaned.

```bash
git worktree list --porcelain | grep '^worktree ' | cut -d' ' -f2- > /tmp/wt-paths.txt
for d in ~/.stapler-squad/shared-node-modules/*/; do
  hash=$(basename "$d")
  referenced=0
  while IFS= read -r wt; do
    [ -L "$wt/web-app/node_modules" ] || continue
    target=$(readlink -f "$wt/web-app/node_modules" 2>/dev/null)
    [ "$target" = "$(readlink -f "$d/node_modules")" ] && referenced=1 && break
  done < /tmp/wt-paths.txt
  if [ "$referenced" = 0 ]; then
    # Skip anything younger than ~1 hour -- a worktree may be mid-install
    # (holding the hash's .lock) and not yet registered its symlink.
    age_secs=$(( $(date +%s) - $(stat -c %Y "$d" 2>/dev/null || stat -f %m "$d") ))
    [ "$age_secs" -lt 3600 ] && continue
    echo "orphaned, no referencing worktree, older than 1h: $d"
    # rm -rf "$d" "${d%/}.lock"  -- uncomment once you've eyeballed the list.
    # Note the ${d%/} strip: $d carries the trailing slash from the `*/`
    # glob, and the per-hash lock file is a SIBLING of the hash dir
    # ("<hash>.lock", created by `exec 200>"$SHARED_ROOT/$HASH.lock"` in
    # shared-node-modules.sh), not a child of it -- "$d".lock would silently
    # target a nonexistent "<hash>/.lock" and never remove the real file.
  fi
done
```

The `.lock` files are tiny (0 bytes, one per hash ever seen) and harmless on
their own, but they'd otherwise accumulate forever with nothing to prune
them — folding their removal into this same step, keyed off the same
orphan/age check as the directory it guards, avoids a second GC mechanism
for a one-line cost.

Same confirmation rule as Step 4: list candidates, get the scope confirmed,
then delete. This is the one GC mechanism for this directory — nothing in
the Go codebase's own sweepers knows it exists (it's pure filesystem state
outside any `Instance`/`Worktree` row), the same reasoning as Step 0 above
for triage worktrees.

## Notes

- Removing a worktree does not delete its branch ref — branches are cheap to
  keep and this skill doesn't touch them. Only revisit branch deletion if
  the user asks separately.
- Worktree paths for this repo aren't all under one directory: check
  `~/.stapler-squad/workspaces/*/worktrees/`, `.claude/worktrees/agent-*`
  (repo-relative), and ad hoc paths like `~/tmp-verify/*` — `git worktree
  list --porcelain` is the only reliable source of truth, not a directory
  listing.

## Bazel output bases

Removing a worktree does not remove its Bazel output base under `~/.cache/bazel/_bazel_$USER/<hash>/`
(bases kept inside a worktree as `.bazel-out` go with `git worktree remove`). Orphan bases are found by their
`DO_NOT_BUILD_HERE` file, which holds the workspace path and no trailing newline.

```bash
bash .claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh          # dry run: lists, removes nothing
bash .claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh --apply  # remove orphan bases
bash .claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh --apply --tmp  # also stale /tmp/ssq-* dirs
```

The dry run also reports install bases older than 30 days (Bazel collects them itself; the script only reports),
the shared disk cache size against its 100 GB GC cap, and stale test temp directories. `--apply` never touches a
base whose workspace exists, the shared disk cache, or the repository cache. `bazel shutdown` cannot be used on an
orphan (its workspace is gone), so the script kills the server named by `server/server.pid.txt` only when
`/proc/<pid>/cmdline` carries `--output_base=<base>`, waits for it to exit, then removes the base.
