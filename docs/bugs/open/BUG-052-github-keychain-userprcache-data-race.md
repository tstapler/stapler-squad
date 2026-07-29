# BUG-052: Data Race Between `SetKeychainTokenForAccount` and `UserPRCache`'s Background Refresh [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-07-29, during CI review of PR #285 (`feat/backlog-category-defaults`), a backlog-automation feature unrelated to the `github` package. Confirmed via `git diff` that PR #285's diff contains zero references to `github_user_service.go`, `github/keychain.go`, or `github/user_pr_cache.go` — not caused by that change.
**Impact**: `make ci`'s `Test` job (`go test -race`) fails intermittently and unpredictably whenever a GitHub-account-adding test runs concurrently with `UserPRCache`'s background refresh loop in the same test binary. Reproduced identically across 4 separate CI runs on PR #285 (always the same test, always the same race), while PR #284's CI run the same day passed clean — consistent with a real, always-latent race whose reproduction depends on `go test`'s scheduling/parallelism, not with flaky test logic. No known production incident yet, but the same unsynchronized access pattern exists in the non-test code path too (see below).

## Problem Description

`go test -race ./server/services/...` intermittently fails `TestAddGitHubAccountWithToken_ValidToken_StoresAndReturnsAccount` with:

```
WARNING: DATA RACE
Write at 0x00c0017c4d80 by goroutine (TestAddGitHubAccountWithToken...):
  github.com/zalando/go-keyring.(*mockProvider).Set()
  github.com/tstapler/stapler-squad/github.SetKeychainTokenForAccount()
      github/keychain.go:95
  github.com/tstapler/stapler-squad/server/services.(*GitHubUserService).AddGitHubAccountWithToken()
      server/services/github_user_service.go:274

Previous read by goroutine (UserPRCache.loop() background goroutine):
  github.com/zalando/go-keyring.(*mockProvider).Get()
  github.com/tstapler/stapler-squad/github.ListKeychainAccounts()
      github/keychain.go:63
  github.com/tstapler/stapler-squad/github.GetAllKeychainTokens()
      github/keychain.go:114
  github.com/tstapler/stapler-squad/github.collectAllTokens()
      github/user_pr_cache.go:474
  github.com/tstapler/stapler-squad/github.(*UserPRCache).resolveAllLogins.func1()
      github/user_pr_cache.go:397
  golang.org/x/sync/singleflight.(*Group).Do()
  github.com/tstapler/stapler-squad/github.(*UserPRCache).resolveAllLogins()
      github/user_pr_cache.go:396
  github.com/tstapler/stapler-squad/github.(*UserPRCache).fetch()
      github/user_pr_cache.go:323
  github.com/tstapler/stapler-squad/github.(*UserPRCache).loop()
      github/user_pr_cache.go:304
```

Root cause: `SetKeychainTokenForAccount` (`github/keychain.go:95`) and `ListKeychainAccounts`/`GetAllKeychainTokens` (`github/keychain.go:63,114`) call directly into the third-party `zalando/go-keyring` package's `Set`/`Get` with no synchronization of their own. Under test, `keyring`'s backend is `mockProvider` (`keyring_mock.go`), which is unsynchronized shared state (confirmed by the race detector) — a `AddGitHubAccountWithToken` RPC call and `UserPRCache`'s background `loop()` → `fetch()` → `resolveAllLogins()` → `collectAllTokens()` refresh cycle (`github/user_pr_cache.go:304→323→396→474`) can run concurrently in the same process and race on it.

This is not purely a test-infrastructure artifact: nothing in `github/keychain.go` itself synchronizes concurrent `Set`/`Get` calls against the real OS keyring backend either (macOS Keychain / Secret Service over D-Bus) — those backends' own thread-safety guarantees (or lack thereof) are external and unverified here. `UserPRCache`'s background refresh loop and an in-flight `AddGitHubAccountWithToken` RPC call are both real, independently-triggerable code paths in production, so the same race shape is plausible there too, just currently only provable under `-race` against the test-only mock backend.

## Suggested Fix (not done here — needs its own root-caused pass)

Guard all `keyring.Set`/`keyring.Get`/`keyring.Delete` calls in `github/keychain.go` (`SetKeychainTokenForAccount`, `ListKeychainAccounts`, `GetAllKeychainTokens`, and any sibling delete/clear function) behind a single package-level `sync.Mutex` (or `sync.RWMutex` if reads clearly dominate and read-read concurrency matters). This serializes access regardless of whether the underlying backend (mock or real OS keyring) is itself thread-safe, which is the right level to fix this at — `github/keychain.go` is the one place all call sites already funnel through.

Route through `sdd:fix-bug`: root-cause-confirm exactly which `keychain.go` functions need the lock (likely all exported functions that call into `keyring.*`), add the mutex, and add a regression test that exercises concurrent `SetKeychainTokenForAccount` + `ListKeychainAccounts`/`GetAllKeychainTokens` under `-race` to prove the fix actually closes the race (not just happens to not reproduce it once).

## Related

- Not related to PR #284 or #285 (the backlog-automation PRs whose CI review surfaced this) — filed as a fast-follow per this repo's convention of documenting adjacent issues found during unrelated work (see `.claude/skills/backlog-feature-improvement`'s own precedent for this pattern, and BUG-051 for the same "flaky CI test, unrelated to the diff that surfaced it" shape applied to a different package).
- Same general class as BUG-051 (`session/tmux` flake) in that both are races/resource-contention issues surfaced by `-race`/parallel `go test`, not logic bugs in the tests themselves — but BUG-051 is resource contention in real tmux server processes under test parallelism, while this is a genuine unsynchronized-shared-state data race in application code (`github/keychain.go`), independent of any test-runner resource limits.
