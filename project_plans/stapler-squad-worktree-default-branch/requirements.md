# Requirements: session start fails resolving default branch (worktreeconfig)

Backlog item 9c33e913-0aa0-4eeb-b6a0-ee33f15ad8a8.

## Problem

Async session start failed: `resolve default branch: ... failed to open git repo at <repo>: core.repositoryformatversion does not support extension: worktreeconfig`.

## Confirmed root cause

`git config --list --show-origin` in the repo shows `.git/config` sets `core.repositoryformatversion=0` and `extensions.worktreeconfig=true` (git adds this extension when a linked worktree uses per-worktree config). Upstream go-git rejects `worktreeconfig` on a V0 repo, so `OpenRepo` fails and default-branch resolution fails.

## Status

Fixed and merged in afbc063dd (#1430): go-git fork `v5.19.3-ssq.3` plus `ops.go` branch-SHA resolution via `backend.ResolveRef`. Full SDD artifacts for that fix: `project_plans/fix-worktreeconfig-extension-compat/` (requirements, research, plan, adversarial reviews, validation, pre-mortem). This directory records the pipeline pass for the remaining open criteria.

## Remaining acceptance criteria

- AC 9: `make test` exits 0. Prior failures were in `session/vc` (jj detection via host `~/.jj`) and `session/tymux`; b6820123c addresses both and must be verified.
- AC 11: no `does not support extension: worktreeconfig` lines in `~/.stapler-squad/logs/staplersquad.log` after deploying a binary built from the fix. Zero matches against the old binary is not proof.

ACs 1-8 and 10 are already marked passing.
