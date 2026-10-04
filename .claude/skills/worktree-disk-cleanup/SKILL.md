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

## Step 1 — Inventory

```bash
git worktree list --porcelain > worktrees.porcelain   # path/HEAD/branch per entry
gh pr list --repo <owner>/<repo> --state all --json number,state,headRefName,mergedAt,updatedAt --limit 1000 > prs.json
```

Parse the porcelain output into `{path, branch, head}` records (skip the
`bare`/main entry). Cross-reference each branch against `prs.json` by
`headRefName`.

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
  whether the diff is trivial (single boilerplate file, SDD planning
  artifacts, CI experiment)
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

## Step 4 — Removal

```bash
git worktree remove --force <path>   # --force needed: junk/dirty files block a plain remove
git worktree prune -v                # clean up stale worktree admin metadata afterward
```

Never touch the `manual_review` or `keep` buckets without the user's
explicit go-ahead — that bucket is where real unshipped work lives. Get
confirmation on removal *scope* (which buckets) before running any
`git worktree remove`, even when every entry in a bucket looks safe — this
is a bulk destructive operation over other people's (or your own past
sessions') work.

## Notes

- Removing a worktree does not delete its branch ref — branches are cheap to
  keep and this skill doesn't touch them. Only revisit branch deletion if
  the user asks separately.
- Worktree paths for this repo aren't all under one directory: check
  `~/.stapler-squad/workspaces/*/worktrees/`, `.claude/worktrees/agent-*`
  (repo-relative), and ad hoc paths like `~/tmp-verify/*` — `git worktree
  list --porcelain` is the only reliable source of truth, not a directory
  listing.
