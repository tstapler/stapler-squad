# BUG-094: `Test_LoadPluginDir`'s colliding-filename winner is nondeterministic under full-suite load [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-09-02, while verifying an unrelated performance-fix branch's test suite
(`go test ./session/... ./server/services/...`)
**Impact**: Test-only. Fails intermittently in the full `session/detection` package run:

```
--- FAIL: Test_LoadPluginDir (3.89s)
    --- FAIL: Test_LoadPluginDir/LoadPluginDir_should_pickSameWinnerEveryTime_When_calledTenTimesOnCollidingFiles (1.10s)
        plugins_test.go:587: iteration 5: winner SourcePath() = ".../z.toml", want it to end in "a.toml"
```

Re-running `go test ./session/detection -run Test_LoadPluginDir -count=1` in isolation 3/3 times passed
cleanly.

## Problem Description

The subtest asserts that `LoadPluginDir` deterministically picks the same winner (by filename, `a.toml`)
across ten repeated calls on a fixture with colliding plugin names. The winner-selection logic in
`session/detection/plugins.go` most likely relies on `os.ReadDir`'s returned order (typically
lexicographic on most filesystems, but not guaranteed by the `os` package contract) rather than an
explicit sort or tie-break rule independent of directory-iteration order. Under load (many concurrent
`go test` processes competing for disk/CPU, as during a full-suite run), directory read ordering or
timing can apparently vary enough to flip the observed winner.

Not root-caused further here — out of scope for the branch being verified (a `session`/`server/services`
performance-fix set unrelated to plugin loading).

## Reproduction Steps

1. `go test ./session/detection/...` under load (e.g. concurrently with other heavy `go build`/`go test`
   invocations) — occasionally fails the colliding-files winner-determinism subtest.
2. `go test ./session/detection -run Test_LoadPluginDir -count=1` in isolation — passed 3/3 in the
   session that discovered this.

## Fix Approach

1. Confirm the hypothesis: check whether `LoadPluginDir`'s collision resolution sorts candidates
   explicitly (e.g. by filename) before picking a winner, or relies on `os.ReadDir` order.
2. If relying on read order, add an explicit deterministic tie-break (e.g. sort candidates
   lexicographically by filename before selecting) so the winner is independent of filesystem iteration
   order and thus of load.
3. Verify: run the full `session/detection` suite in a loop under artificial CPU load (e.g. alongside
   `go build ./...` in a loop) and confirm the winner never flips.

## Related

- `.claude/rules/fix-flaky-tests-dont-defer.md` — filed per this repo's standing rule rather than
  silently re-excused, since bisecting `os.ReadDir` ordering guarantees was out of scope for the branch
  that surfaced it.
