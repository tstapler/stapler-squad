# Blocked: backlog item 65de9ebc-0868-4ab8-9fca-67666a40fe13

Fixing the `Instance.status` and `redirectInfoLog` data races requires a checkout of the
target application repo (the one containing `go.mod`, `session/state_machine_test.go`,
`session/instance_state.go`, `session/backlog_lifecycle_test.go`, `log/log.go`, etc.).

This worktree does not have that repo. Evidence:

- `git log --oneline` shows a single commit, `700777f Initial commit`, containing only
  `.gitignore` (plus `.claude/` unstaged/managed separately).
- `find . -maxdepth 3` from the worktree root returns no source directories — no `go.mod`,
  no `session/`, no `log/`.
- `git branch -a` shows only `master`; `git remote -v` is empty; `git merge main` fails
  with "main - not something we can merge".
- The actual `stapler-squad` repo exists locally at
  `~/code/github.com/tstapler/stapler-squad` (found via `mdfind`), but that is the user's
  own primary checkout with unrelated uncommitted changes — not this worktree, and not
  something a triage session should mutate directly.
- A sibling worktree in this same workspace
  (`stapler-squad-fix-cgo-release-sqlite-driver_18cb3f76ddc143d8`) hit and documented the
  identical problem in its own `BLOCKER-3353c776.md`: empty scaffold repo, no source tree,
  called out as "an infrastructure/workspace-provisioning gap, not something fixable from
  inside the session."

None of acceptance criteria 0-4 (fix `Instance.status` test read, consolidate logger-swap
helpers, confirm/fix the timeout panic, `go vet`/`make lint`, `t.Parallel()` verification)
can be attempted without an actual checkout of `session/`, `log/`, and the module root in
this worktree. This is a recurring workspace-provisioning gap affecting multiple worktrees
in workspace `6eb0b580fa0331d5`, not something fixable from inside the session.

This file exists solely to give the review system a committed artifact documenting the
blocker investigation; it should be deleted once the worktree is correctly provisioned and
the real fix work can start.
