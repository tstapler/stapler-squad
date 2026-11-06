# Verification Evidence

## Repro fails without the fix (criterion 1)
`ReserveReview` in `spawnReviewGate` was replaced with a no-op reservation, then:
`go test -race ./session -run TestReviewGate_ConcurrentTriggers_SingleSpawn -count=1`
→ FAIL, `expected: 1 actual: 3` ("exactly one review ItemSession row"). Change reverted; with the guard the test passes.

## Tests with the fix
`go test -race ./session -run 'ReviewSpawn|ReserveReview|Reproduc' -count=1` → ok
`go test -race ./session ./server/services -run 'Review|Backlog' -count=1` → ok (both packages)

## Lint / gates (criterion 8)
- `golangci-lint run --new-from-rev=$(git merge-base HEAD origin/main) ./session/... ./server/...` → 0 issues; `go vet` clean.
- `make ready` cannot complete in this worktree: it aborts at `build-tmux` (`fatal: Unable to find current revision in submodule path 'third_party/tmux'`), an environment problem unrelated to the diff.
- `make ready-complexity-gate` reports only `server/services/session_service.go` findings (dupl, gocognit, file length); that file is not in `git diff origin/main...HEAD`, and local `origin/main` is 4 commits ahead of this branch.

## Architecture / idiom review (sdd:6 equivalent)
Single mutex over the whole reservation map (not per-item), idempotent release deferred at each call site, reservation held across `runner.Run` which persists the review ItemSession. Guard instance shared with `BacklogService` via `SetReviewSpawnGuard`; nil-safe.
