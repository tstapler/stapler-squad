# ADR-001: Fork the v5 line (v5.19.x), not v6

**Status**: Proposed (confirmed or reversed by spike gate G2)
**Date**: 2026-10-08

## Context
- The repo pins `github.com/go-git/go-git/v5 v5.19.2` and `go-billy/v5 v5.9.0` (`go.mod:30,149`); 30 non-test files import go-git (`git grep -lE 'go-git/go-git/v5' -- '*.go' ':!*_test.go' ':!.claude' | wc -l` = 30).
- Upstream linked-worktree add/list/remove-metadata exists only in v6 `x/plumbing/worktree` (experimental). v6.0.0-beta.1 shipped 2026-10-04; there is no stable v6. v5.19.3 (stable) shipped the same day. (research/stack.md section 1, VERIFIED there.)
- The repo already implements worktree add/remove/prune/list in-process on v5 (`session/git/native_worktree_*.go`, #730), so the one thing v6 uniquely offers is already solved here.
- Upstream security policy supports only the latest minor of each line (research/pitfalls.md 1.1).

## Decision
Fork `go-git/go-git` at tag `v5.19.3` into `github.com/tstapler/go-git`, branch `ssq/v5`. Keep every patch additive (new files or new packages, minimal edits to hot upstream files) so it can be ported to v6 later. Track the latest v5 minor on every upstream release.

Re-evaluation triggers (checked at gate G2 and at each rebase): v6.0.0 stable has shipped and v5 has had no release for 90 days, or upstream announces v5 end-of-life. Then open a follow-up ADR for a v6 port; the additive-patch rule is what keeps that port bounded.

## Consequences
- No import-path migration (30 files, billy v6) and no dependence on an `x/` API that "may change without notice".
- We carry the burden of anything v6 fixes that is not backported to v5 (the v5 line still receives security fixes while it is the latest stable).
- Patches that depend on v6-only storage changes are out of reach; none are in the plan.

## Alternatives rejected
- **Fork v6 main**: tracks upstream's future, but a beta moving target; forces module-path and billy v6 migration across 30 files plus tests; duplicates the already-working native worktree code.
- **Adopt v6 `x/plumbing/worktree` unforked**: its `Remove` deletes metadata only, `Add` has a Lstat-then-Mkdir race that `native_worktree_add.go` already avoids, and there is no prune/lock/move.
