# BUG-100: `KeyringTokenSource.runProbe`'s `raceKeyringOp` goroutine can outlive its test and race `swapKeyringSeams`' cleanup [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-09-04, running `go test -race -count=1 ./...` repeatedly (5x) to verify backlog item
`e271db3d-7e79-48af-a1e7-96df54d8d600` (UserPRCache.Stop()/StreamTerminal flakes) did not regress the rest
of the suite. Reproduced once in 5 repeated full-suite runs.

## Problem Description

```
WARNING: DATA RACE
Read at 0x000001817610 by goroutine 86:
  jules.(*KeyringTokenSource).runProbe.func1()
      jules/keychain.go:255
  jules.(*KeyringTokenSource).raceKeyringOp.func1()
      jules/keychain.go:347

Previous write at 0x000001817610 by goroutine 82:
  jules.swapKeyringSeams.func1()
      jules/keychain_test.go:26
  testing.(*common).Cleanup.func1()
```

`raceKeyringOp` (`jules/keychain.go:337-354`) launches an unbounded inner goroutine to call the injected
`keyringGet`/`keyringSet`/`keyringDelete` package-level test seam, then races it against a
`context.WithTimeout`. If the timeout wins, `raceKeyringOp` returns immediately — but the inner goroutine
is still running and will still call the (package-level, not per-instance) `keyringGet` var some time
later. `runProbe` (`keychain.go:254`) calls this from a bare `go s.runProbe()` (`keychain.go:167`)
triggered by `APIKey()` when the circuit is open, with no join/wait exposed to callers. If the owning
test returns and `t.Cleanup` (`swapKeyringSeams`, `keychain_test.go:22-26`) restores the original
`keyringGet`/`Set`/`Delete` vars while that leaked goroutine is still mid-call, the two race on the same
package-level var.

## Why this looks familiar

This is the same goroutine-outlives-its-owner class of bug as `UserPRCache.Stop()` (already fixed,
`github/user_pr_cache.go:161-168`, this backlog item's own fix) and `AnalyticsStore.Stop()`
(`server/services/analytics_store.go`) — a background goroutine touching shared/package-level state
with no completion signal a caller (or a test's cleanup) can wait on before tearing down.

## Fix Approach (not attempted here — out of scope for this item)

`raceKeyringOp`/`runProbe` need the same fix shape: either (a) a way for `swapKeyringSeams`' cleanup to
wait for any in-flight probe to finish before restoring the seams (a `sync.WaitGroup` or done-channel
tracked on `KeyringTokenSource`), or (b) route `keyringGet`/`Set`/`Delete` access through a mutex
(mirroring `github/keychain.go`'s `keychainMu` — see that file's comment on why per-key locking doesn't
suffice for a shared mock store) so the race detector no longer flags the interleaving even though the
seam-swap timing is still logically racy.

## Related

- `fix-flaky-tests-dont-defer` skill — filed per this repo's standing rule rather than re-excusing.
- Same family as `github.UserPRCache.Stop()` (fixed) and BUG-084/090/092 (goroutine-outlives-test flakes).
