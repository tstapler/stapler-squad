# BUG-095: `TestSetupNewWorktree_SelfHeals_When_BranchCreatedByDelayedRaceWinner` flaky under load [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-09-02, while verifying an unrelated performance-fix branch's test suite
(`go test ./session/... ./server/services/...`)
**Impact**: Test-only. Fails intermittently in `session/git` (~1 in 5 runs when repeated back-to-back
under load); passes the large majority of the time.

## Problem Description

The test simulates a "delayed race winner" scenario: a concurrent branch-create call wins the race after
this process already observed the branch didn't exist, and `SetupNewWorktree` is expected to self-heal
by detecting the branch now exists and reusing it rather than failing. Log output from a failing run
showed:

```
WARN could not find merge-base for branch with any default branch (main/master/develop/trunk) branch=backlog/delayed-race-winner-layer1
```

immediately after the self-heal path reused the branch — suggesting a timing-sensitive interaction
between the self-heal retry and merge-base resolution against the fixture repo's default branches, not
a logic defect in the self-heal detection itself (the branch reuse succeeded; only the downstream
merge-base lookup warned).

## Reproduction Steps

Repeat back-to-back under load:

```
for i in 1 2 3 4 5; do go test ./session/git -run TestSetupNewWorktree_SelfHeals_When_BranchCreatedByDelayedRaceWinner -count=1 -timeout 30s; done
```

Observed 1 failure in 5 runs (2026-09-02, alongside several other heavy concurrent `go build`/`go test`
processes). Not reproduced in a subsequent full-suite run under lighter load.

## Fix Approach

1. Reproduce reliably under controlled artificial load (e.g. `GOMAXPROCS`-limited or run alongside a
   background CPU-burn loop) to get a consistent repro rate.
2. Once reproducible, check whether the merge-base lookup races the self-heal's branch-create commit
   becoming visible to the merge-base-resolving subprocess/go-git read (a torn-read class issue, similar
   in shape to `session/git/util.go`'s documented `getHeadCommitSHA` torn-read mitigation for worktrees).
3. If confirmed, apply the same retry-with-verification pattern already used for `getHeadCommitSHA`
   rather than widening a timeout.

## Related

- `.claude/rules/fix-flaky-tests-dont-defer.md` — filed per this repo's standing rule rather than
  silently re-excused, since deep root-causing was out of scope for the branch that surfaced it.
- `session/git/util.go`'s `getHeadCommitSHA` doc comment — prior art for a very similar
  go-git-vs-concurrent-git-operation torn-read class of issue in this same package.
