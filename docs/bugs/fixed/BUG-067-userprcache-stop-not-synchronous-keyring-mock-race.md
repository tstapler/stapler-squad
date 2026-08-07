# BUG-067: `UserPRCache.Stop()` Returns Before Its Background Goroutine Exits, Racing the Next Test's `keyring.MockInit()` [SEVERITY: Medium]

**Status**: ✅ Fixed
**Discovered**: 2026-08-07, `main` branch CI (`Test` job, run [31131559730](https://github.com/tstapler/stapler-squad/actions/runs/31131559730/job/92722008058)) — `go test -race` failed on `server/services` with a real data race, not a flake in the harness.

## Live Evidence

```
WARNING: DATA RACE
Write at 0x000005ca95e0 by goroutine 23741:
  github.com/zalando/go-keyring.MockInit()
      keyring_mock.go:64
  github.com/tstapler/stapler-squad/server/services.newTestGitHubUserService()
      server/services/github_user_service_test.go:21
  github.com/tstapler/stapler-squad/server/services.TestListGitHubAccounts_AccountOnUnconfiguredEnterpriseHost_IncludesHostInEnterpriseHosts()

Previous read at 0x000005ca95e0 by goroutine 23740:
  github.com/zalando/go-keyring.Get()
  github.com/tstapler/stapler-squad/github.keyringGet()
      github/keychain.go:36
  github.com/tstapler/stapler-squad/github.GetAllKeychainTokens()
      github/keychain.go:184
  github.com/tstapler/stapler-squad/github.collectAllTokens()
      github/user_pr_cache.go:474
  github.com/tstapler/stapler-squad/github.(*UserPRCache).resolveAllLogins.func1()
  ... singleflight ...
  github.com/tstapler/stapler-squad/github.(*UserPRCache).fetch()
  github.com/tstapler/stapler-squad/github.(*UserPRCache).loop()
      github/user_pr_cache.go:144

Goroutine 23740 (finished) created at:
  github.com/tstapler/stapler-squad/server/services.newTestGitHubUserService.(*UserPRCache).Start.func1()
      github/user_pr_cache.go:144
  github.com/tstapler/stapler-squad/server/services.TestAddGitHubAccountWithToken_EmptyToken_ReturnsInvalidArgument()
```

A background `UserPRCache` goroutine spawned by one test (`TestAddGitHubAccountWithToken_EmptyToken_ReturnsInvalidArgument`) was still reading the global `go-keyring` mock state while a *different*, later test (`TestListGitHubAccounts_...`) called `keyring.MockInit()` to reset it.

## Root Cause

`server/services/github_user_service_test.go:19-26`'s `newTestGitHubUserService(t)` helper does, for every test:

```go
keyring.MockInit()
cache := gh.NewUserPRCache()
cache.Start(context.Background())
t.Cleanup(cache.Stop)
```

`UserPRCache.loop()` (`github/user_pr_cache.go:302`) calls `c.fetch()` **synchronously, before** entering its `select { case <-c.ctx.Done(): ... }` loop — so the very first fetch always runs regardless of how soon `Stop()` is called afterward. `fetch()` calls into `resolveAllLogins()` → `collectAllTokens()` → `GetAllKeychainTokens()` → `keyringGet()` → `keyring.Get()`, none of which check `ctx.Done()`.

`Stop()` (`github/user_pr_cache.go:149`, before this fix) was:

```go
func (c *UserPRCache) Stop() {
	c.cancel()
}
```

`cancel()` only signals the context — it does not wait for `loop()` to observe the signal or for any in-flight synchronous call inside it to return. So `t.Cleanup(cache.Stop)` could return while the goroutine's initial `fetch()` was still mid-flight, reading the global (package-level) keyring mock state. The very next test in the same test binary calling `newTestGitHubUserService(t)` then called `keyring.MockInit()` — a write to that same global state — while the previous test's leftover goroutine was still reading it. Classic non-synchronous shutdown: `Stop()`'s name and doc comment ("halts background polling") implied a synchronous guarantee it didn't actually provide.

## Fix

Added a `done chan struct{}` to `UserPRCache`, closed via `defer close(c.done)` at the top of `loop()`, and made `Stop()` wait for it:

```go
func (c *UserPRCache) Stop() {
	c.cancel()
	<-c.done
}
```

`Stop()` now only returns once `loop()` — including any fetch already in flight — has fully exited, so no goroutine spawned by `Start()` can still be running (or touching shared/global state) after `Stop()` returns. This matches how every current call site is already used (tests always pair `Start()` with `Stop()`; production (`server/server.go:415`) only calls `Start()` and never `Stop()`, so this doesn't change production shutdown behavior).

## Regression Test

`github/user_pr_cache_internal_test.go` (new, white-box `package github` to reach the unexported `done` field):

- `TestUserPRCache_Stop_WaitsForLoopGoroutineToExit` — starts a cache, calls `Stop()`, and asserts `c.done` is already closed by the time `Stop()` returns. This is a deterministic ordering check (not a timing-dependent repro of the original race), directly encoding the invariant the fix establishes.

`go test -race ./github/...` (all green), `go test -race -count=5 ./server/services/... -run 'TestListGitHubAccounts_AccountOnUnconfiguredEnterpriseHost_IncludesHostInEnterpriseHosts|TestAddGitHubAccountWithToken'` (all green, 5 iterations, no race), `go build ./...` (clean), `golangci-lint run ./github/...` (0 issues).

## Phase D — Classification

**Classification**: API Contract Gap. `Stop()`'s name/doc comment promised synchronous shutdown ("halts background polling") that the implementation didn't provide — a mismatch between the documented contract and the actual guarantee, only surfaced by `-race` under the specific interleaving of two tests sharing a process-global dependency (`go-keyring`'s package-level mock state).

**Earliest enforcement point**: The regression test (a deterministic channel-close assertion) is the earliest achievable level for this specific case — the underlying invariant ("no `Start`ed goroutine survives past `Stop()` returning") isn't expressible as a compile-time or lint check without a much larger effort (e.g. a linter that tracks goroutine lifetimes against `sync.Once`-gated struct fields), so a unit test is proportionate here.

**Recurring shape**: A `Start`/`Stop` pair where `Stop` cancels a context but doesn't join the goroutine it started is a generic concurrency footgun, not unique to this file — worth a quick grep (`grep -rn "func.*Stop()" --include=*.go`) next time a similar background-cache pattern is added, to check whether its `Stop()` actually blocks or just requests. Not filing a repo-wide sweep here since only this one call site was reproduced by CI.

## Related

- Found while investigating "fix the CI race on main" per the `.claude/rules/fix-flaky-tests-dont-defer.md` discipline — root-caused and fixed in the same session rather than re-excused as "known flake."
- Distinct from the separately-tracked `TmuxSession.ptmx` data race (backlog item `c42de545-ee23-420f-950b-d7635ab6ae27`, in progress in another session) — that's an unguarded field access in `session/tmux/tmux.go`, unrelated code path.
