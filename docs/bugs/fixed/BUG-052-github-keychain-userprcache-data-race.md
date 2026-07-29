# BUG-052: Data Race Between `SetKeychainTokenForAccount` and `UserPRCache`'s Background Refresh [SEVERITY: Medium]

**Status**: ✅ FIXED (2026-07-29)
**Discovered**: 2026-07-29, during CI review of PR #285 (`feat/backlog-category-defaults`), a backlog-automation feature unrelated to the `github` package. Confirmed via `git diff` that PR #285's diff contains zero references to `github_user_service.go`, `github/keychain.go`, or `github/user_pr_cache.go` — not caused by that change.
**Fixed**: 2026-07-29 — `github/keychain.go` (see `sdd:fix-bug` session; PR TBD against `tstapler/stapler-squad`)
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

## Suggested Fix (as originally filed)

Guard all `keyring.Set`/`keyring.Get`/`keyring.Delete` calls in `github/keychain.go` (`SetKeychainTokenForAccount`, `ListKeychainAccounts`, `GetAllKeychainTokens`, and any sibling delete/clear function) behind a single package-level `sync.Mutex` (or `sync.RWMutex` if reads clearly dominate and read-read concurrency matters). This serializes access regardless of whether the underlying backend (mock or real OS keyring) is itself thread-safe, which is the right level to fix this at — `github/keychain.go` is the one place all call sites already funnel through.

## Fix Applied

Confirmed via a full re-read of `github/keychain.go` that **every** `keyring.*` call in the package funnels through 11 call sites across 8 exported/unexported functions (`GetKeychainToken`, `SetKeychainToken`, `DeleteKeychainToken`, `ListKeychainAccounts`, `GetKeychainTokenForAccount`, `SetKeychainTokenForAccount`, `DeleteKeychainTokenForAccount`, `GetAllKeychainTokens`, `addToAccountList`, `removeFromAccountList`) — matching the bug's own hypothesis that all call sites are already in this one file.

Implemented as a package-level `sync.Mutex` (`keychainMu`) plus three private wrapper functions — `keyringGet`, `keyringSet`, `keyringDelete` — each locking `keychainMu` around a single `keyring.Get`/`Set`/`Delete` call. Every direct `keyring.*` call site in the file was replaced with the corresponding wrapper.

**Why wrappers instead of locking inside every exported function directly**: several of this package's functions call each other (e.g. `GetKeychainToken` calls `ListKeychainAccounts` and `GetKeychainTokenForAccount`; `SetKeychainTokenForAccount` calls `addToAccountList`, which itself calls `ListKeychainAccounts`; `DeleteKeychainTokenForAccount` calls `removeFromAccountList`, likewise). Locking `keychainMu` at the top of each of *those* functions would self-deadlock on the nested calls with a non-reentrant `sync.Mutex`. Pushing the lock down to the three leaf-level keyring wrappers instead means the mutex is only ever held for the duration of one `keyring.Get`/`Set`/`Delete` call — never across a multi-step outer function — so there is no nesting and no deadlock risk, while every individual keyring access is still fully serialized against every other one.

**Plain `sync.Mutex`, not `sync.RWMutex`**: confirmed via call-site frequency that keychain access is not a hot path — reads happen once per `UserPRCache` poll interval (config-driven, default on the order of tens of seconds to minutes) plus occasional user-triggered RPC calls (add/list/remove account). Read-read concurrency has no measurable throughput benefit at this call frequency, so the extra complexity of `RWMutex` (and its own subtler correctness footguns) isn't justified — this matches the bug doc's own caveat to only reach for `RWMutex` if reads clearly dominate and concurrency matters, which was checked and found not to apply.

Per `.claude/rules/interface-pollution-checklist.md`, no new interface or wrapper type was introduced — `keychainMu` is a plain unexported field-equivalent (package-level `sync.Mutex`) guarding existing package-level behavior, and the three wrapper functions are the minimal leaf-level guard needed to avoid the nested-lock deadlock described above, not a speculative abstraction.

## Regression Tests

Added `github/keychain_test.go`:

- `TestSetKeychainTokenForAccount_NoRace_When_ConcurrentWithListAndGetAllTokens` — 20 goroutines calling `SetKeychainTokenForAccount` concurrently with 20 goroutines calling `ListKeychainAccounts`/`GetAllKeychainTokens` (the exact read paths in the original race trace), then asserts every writer's token actually landed (proving the mutex serializes without silently dropping writes).
- `TestDeleteKeychainTokenForAccount_NoRace_When_ConcurrentWithReads` — same shape for the delete path (`DeleteKeychainTokenForAccount`/`removeFromAccountList`), which uses the same previously-unguarded call sites but wasn't in the original trace.

**Verified to fail against pre-fix code**: temporarily neutered `keychainMu.Lock()/Unlock()` in the three wrapper functions (keeping the new tests as-is) and reran `go test ./github/... -race -count=5`. Reproduced the exact race from the bug report (`mockProvider.Set` write vs. `mockProvider.Get`/`Delete` read, same file:line shape) plus, on the delete test, an outright `fatal error: concurrent map read and map write` crash — confirming the test genuinely exercises the unsynchronized path rather than merely failing to reproduce it by luck. Restored the real fix; reran — clean.

## Verification (post-fix)

```
$ go build ./github/... ./server/services/...
(clean)

$ go test ./github/... ./server/services/... -race
ok  	github.com/tstapler/stapler-squad/github	1.060s
ok  	github.com/tstapler/stapler-squad/server/services	393.007s

$ go test ./server/services/... -run TestAddGitHubAccountWithToken_ValidToken_StoresAndReturnsAccount -race -count=5 -v
=== RUN   TestAddGitHubAccountWithToken_ValidToken_StoresAndReturnsAccount
--- PASS: TestAddGitHubAccountWithToken_ValidToken_StoresAndReturnsAccount (0.00s)
(x5, all PASS, no race warnings)
PASS
ok  	github.com/tstapler/stapler-squad/server/services	1.067s

$ go test ./github/... -run 'TestSetKeychainTokenForAccount_NoRace|TestDeleteKeychainTokenForAccount_NoRace' -race -count=5 -v
(x5 each, all PASS, no race warnings)
PASS
ok  	github.com/tstapler/stapler-squad/github	1.049s
```

`golangci-lint run --enable=nilnil,staticcheck,ineffassign,govet ./github/... ./server/services/...` → 0 issues.
Custom project linter (`bin/linter ./github/... ./server/services/...`) → 0 issues, exit 0.

`make lint` itself could not be run end-to-end in this dev environment: its `lint-custom` step depends on `go list ./...`, which fails on the pre-existing, unrelated `server/web/embed.go:8: pattern all:dist: no matching files found` (the web UI dist bundle is not built in this environment — the same gap already documented in `docs/bugs/fixed/BUG-048-...md`'s Verification section). Both lint stages were instead run directly, scoped to the two packages this fix touches (see above), both clean.

## Related

- Not related to PR #284 or #285 (the backlog-automation PRs whose CI review surfaced this) — filed as a fast-follow per this repo's convention of documenting adjacent issues found during unrelated work (see `.claude/skills/backlog-feature-improvement`'s own precedent for this pattern, and BUG-051 for the same "flaky CI test, unrelated to the diff that surfaced it" shape applied to a different package).
- Same general class as BUG-051 (`session/tmux` flake) in that both are races/resource-contention issues surfaced by `-race`/parallel `go test`, not logic bugs in the tests themselves — but BUG-051 is resource contention in real tmux server processes under test parallelism, while this is a genuine unsynchronized-shared-state data race in application code (`github/keychain.go`), independent of any test-runner resource limits.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: API Contract Gap. `zalando/go-keyring`'s package-level `Get`/`Set`/`Delete` functions carry no documented thread-safety guarantee, and the real OS backends behind them (macOS Keychain, Secret Service over D-Bus) are external, unverified black boxes from this codebase's point of view. `github/keychain.go` treated the dependency as if it were safe for concurrent use without ever having confirmed that contract — a classic case of assuming a third-party API's implicit behavior instead of verifying and enforcing the actual one.

**Earliest achievable enforcement**: The `-race`-detected regression tests are the earliest practical level here. This isn't a type-system-expressible invariant (Go's type system has no notion of "this function is safe for concurrent use"), and no lint rule can generically detect "calls into a package without documented thread-safety from multiple goroutines" without either an allow/denylist of specific third-party APIs or full points-to/escape analysis most linters don't do. A `go vet`-style custom check that flags any package-level function shared across two or more goroutine-spawning call sites without an accompanying mutex is theoretically possible but would be high-noise/low-precision for a single, narrow case like this — not implemented here as disproportionate to the risk.

**Recurring shape**: None identified — this is the first instance in this codebase's bug history of "unsynchronized access to a third-party dependency with an implicit (undocumented) thread-safety contract," as distinct from BUG-051's tmux *process*-level resource contention. Worth naming for future audits: when introducing a new package-level (not per-instance) call into a third-party client library from more than one independently-triggerable code path (an RPC handler and a background loop, in this case), check the library's own concurrency documentation before assuming safety, and default to guarding at the call site that owns the shared package-level state — this codebase's `github` package — rather than trusting the dependency.
