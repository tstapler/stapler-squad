# Requirements: backlog-scaffolding-gitignore

**Backlog item**: `109fb4de-a51b-414b-8116-300002d8db24` — "Backlog session scaffolding
(.backlog-context.md, .claude/commands/backlog/*) gets committed into target repos"

**Date**: 2026-08-10

## Note on prior work

A prior triage session already ran this exact SDD pipeline on this exact item and
produced `implementation/validation.md` (committed in `c0bf31a69`), but its
`requirements.md`, `research/*.md`, and `implementation/plan.md` were never committed
and are gone — lost when that session's worktree was cleaned up, the precise failure
mode `.claude/rules/sdd-planning-artifacts-commit.md` warns about. The surviving
`validation.md` references a plan built around `addWorktreeExcludes` resolving
`$GIT_COMMON_DIR` via `--git-common-dir` instead of `--git-dir`, and a new
`GetWorktreeDirtyPaths` / `formatDirtyPathsRejectionMessage` pair. This session
independently re-derived and empirically verified that same root cause before reading
the salvaged file, so the diagnosis below is confirmed via direct testing, not just
inherited from the lost plan.

## Current state (verified against this repo's actual code, not assumed)

Since the item was filed, part of the fix has already shipped:

- `session/git/scaffolding.go` — `ScaffoldingExcludePatterns` (single source of truth:
  `.backlog-context.md`, `.claude/commands/backlog/`, `web-app/.next/`) and
  `UntrackScaffolding` (go-git index surgery that untracks any already-committed
  scaffolding path — "self-heal").
- `session/backlog_commands.go` — `selfHealWorktreeScaffolding` calls
  `UntrackScaffolding` then `addWorktreeExcludes` at the start of every scaffolding
  write.
- `session/git/worktree_git.go` — `StageAllExceptScaffolding` guards the automated
  commit path (`git add .` then untrack scaffolding before `git commit`), with a CI
  backstop workflow (`.github/workflows/backlog-scaffolding-guard.yml`) as a second
  layer.
- This repo's own root `.gitignore` (lines 104–105) lists both scaffolding patterns —
  irrelevant to target repos (that's the whole reason the info/exclude mechanism
  exists instead of writing to the target repo's `.gitignore`), but confirms the
  intent is already understood project-wide.

**What is still broken, confirmed by direct reproduction (see Research):**
`addWorktreeExcludes` (`session/backlog_commands.go:244`) resolves the exclude file
via `git rev-parse --git-dir`. For the linked-worktree topology this project actually
uses (`git worktree add`), `--git-dir` returns the worktree-private admin directory
(`<common>/.git/worktrees/<name>`), and git silently does **not** honor an
`info/exclude` written there — only the one under `--git-common-dir`
(`<common>/.git/info/exclude`) is read by `git status`. Reproduced directly with a
throwaway `git worktree add` fixture: writing the pattern to the `--git-dir` exclude
file left the file `??` (untracked) in `git status --porcelain`; the identical pattern
written to the `--git-common-dir` exclude file made the same worktree report clean.

Net effect: `addWorktreeExcludes` has been a silent no-op for every real (linked)
worktree session since it shipped. Scaffolding files still show up as untracked in
`git status`, `IsWorktreeDirty` (`session/backlog_review.go:626`, which just checks
`len(strings.TrimSpace(git status --porcelain output)) > 0`) still returns `true`,
and `request_review`'s belt-and-suspenders check
(`server/mcp/tools_backlog.go:381-392`) still rejects with the unchanged message:

> `request_review rejected: the worktree has uncommitted changes. Run `git add -A &&
> git commit -m 'description of changes'` to commit your work, then call
> request_review again.`

— i.e. the exact bug this item reports is still live today, via a different failure
point than originally suspected (the exclude mechanism exists but doesn't take effect,
rather than not existing at all).

Also still true, independent of the above: no function exists yet to enumerate the
*specific* dirty paths (only the boolean `IsWorktreeDirty`), so the rejection message
can't distinguish "you have real uncommitted work" from "only scaffolding files are
dirty" even once the exclude bug is fixed — a fallback layer for whatever future case
still lets a scaffolding file slip past `info/exclude` (e.g. a non-worktree
directory-mode session, or an untracked-but-not-yet-excluded file mid-write).

## Scope

**In scope:**
1. Fix `addWorktreeExcludes` to resolve the exclude file via `--git-common-dir`
   instead of `--git-dir`, so the mechanism actually takes effect for linked
   worktrees (this project's real topology).
2. Add a way to get the specific list of dirty paths from a worktree (not just the
   `IsWorktreeDirty` boolean), and use it to build `request_review`'s rejection
   message so it lists the actual offending paths instead of a blanket
   `git add -A` instruction.
3. Regression coverage: a test against a real `git worktree add` fixture that fails
   before the `--git-common-dir` fix and passes after, so this can't silently regress
   again (this is the second time it's been diagnosed).

**Out of scope / not doing:**
- Moving scaffolding write location outside the worktree entirely (the alternative
  suggested in the item body) — the existing exclude + self-heal + commit-time-guard
  architecture is sound in design and mostly already shipped; it only needs the
  `--git-dir` → `--git-common-dir` fix to actually work. Rearchitecting where
  scaffolding lives is a bigger, unjustified change given the existing design is one
  line away from correct.
- Changing `IsWorktreeDirty`'s underlying mechanism away from shelling out to
  `git status --porcelain` (go-git's `Worktree.Status()` is a plausible alternative
  but out of scope here — no evidence it's needed to fix this bug).
- Non-worktree (directory-mode) sessions: `addWorktreeExcludes` already no-ops
  gracefully there (see `UntrackScaffolding`'s doc comment on returning `(nil, nil)`
  for non-git dirs); no behavior change intended for that path.

## Acceptance Criteria

0. `addWorktreeExcludes` writes the scaffolding exclude patterns to the git-common
   `info/exclude` file (resolved via `git rev-parse --git-common-dir`, not
   `--git-dir`), so they are actually honored by `git status` in a real linked
   worktree.
1. A regression test using a real `git worktree add` fixture (not a plain repo)
   demonstrates: writing `.backlog-context.md` into the worktree, running
   `addWorktreeExcludes`, then checking `git status`/`IsWorktreeDirty` reports the
   worktree clean.
2. A function exists that returns the specific list of dirty paths in a worktree
   (not just a boolean), correctly handling untracked files, modified files, and
   renames (existing rename-parsing correctness must be preserved/reused, not
   reimplemented naively).
3. `request_review`'s rejection message, when the worktree is genuinely dirty, lists
   the specific dirty path(s) instead of a blanket `git add -A && git commit`
   instruction.
4. Plain (non-worktree) repos and directory-mode (non-git) sessions see no behavior
   change: existing `addWorktreeExcludes`/`selfHealWorktreeScaffolding` tests
   (`session/backlog_commands_test.go`) continue to pass unmodified.
5. `go build ./... && make test` (or at minimum `go test ./session/... ./server/mcp/...`)
   passes.

## Success Metrics

- A fresh backlog session in a real (linked) worktree that writes only scaffolding
  files no longer trips `request_review`'s uncommitted-changes rejection.
- If `request_review` ever does reject for a genuinely dirty worktree, the message
  names the actual dirty path(s) rather than blindly suggesting `git add -A`.
