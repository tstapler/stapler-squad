# Blocked: backlog item "Fix data race on global log.WarningLog swapped unsynchronized by session tests"

Fixing the `log.WarningLog`/`InfoLog`/`ErrorLog`/`DebugLog` data race requires a checkout of
the target application repo (the one containing `go.mod`, `log/log.go`, `session/review_gate.go`,
`session/pipeline_engine_test.go`, `server/services/*`, etc.).

This worktree does not have that repo. Evidence:

- `git log --all --oneline` shows only `700777f Initial commit` (a `.gitignore` only) plus a
  prior blocker-documentation commit — no application source.
- `find . -maxdepth 2 -name go.mod` returns nothing; there is no `session/`, `log/`, or
  `server/` directory anywhere in this worktree.
- `git branch -a` shows only this backlog branch, a sibling triage branch, and `master` — no
  `main`. `git remote -v` is empty. `git merge main` fails with "main - not something we can
  merge", confirming there is no upstream history to pull the real repo from.
- An identical gap was already documented in this same workspace
  (`BLOCKER-65de9ebc.md`, committed here at 62742d3) for a different backlog item
  (`session/state_machine_test.go` / `redirectInfoLog`), and that file explicitly calls out a
  sibling worktree (`stapler-squad-fix-cgo-release-sqlite-driver_18cb3f76ddc143d8`) hitting the
  same "empty scaffold repo, no source tree" problem — this is a recurring
  workspace-provisioning gap in workspace `6eb0b580fa0331d5`, not something fixable from inside
  a session.
- The actual `stapler-squad` repo does exist locally at
  `~/code/github.com/tstapler/stapler-squad`, but that is the user's own primary checkout with
  unrelated uncommitted changes — not this worktree, and not safe for a triage session to mutate
  directly.

None of acceptance criteria 0-5 (atomic-pointer-backed logger accessors, migrating ~544
production read sites, fixing five unsynchronized test-swap sites, `go build ./...`, and a
regression test) can be attempted without an actual checkout of `log/`, `session/`,
`server/services/`, and the module root in this worktree.

This file documents the blocker investigation for the review system. Delete it once the
worktree is correctly provisioned and the real fix work can start.
