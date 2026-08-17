# BUG-080: `github.DefaultRateLimiter` package-level global leaks rate-limit state across tests, causing full-suite-only CI failures [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-08-17 (during dynamic-rule-reload PR #538's Gate 4 CI run)
**Impact**: Intermittent CI failures on the `Test` job (full coverage run) — 4 tests in
`session/backlog_plugin_github_test.go` fail only when a specific earlier test in the same
binary run has executed first, not when run in isolation. Same class of issue as BUG-076
(package-level shared state bleeding between tests), one layer up — this time a genuine
`var ... = &T{}` package singleton, not incidental shared fixture state.

## Problem Description

CI run (PR #538, https://github.com/tstapler/stapler-squad/actions/runs/32001637582/job/95303403214)
failed with:

```
--- FAIL: TestGitHubIssuesPlugin_CloseIssue_ReturnsResponseUpdatedAt (0.00s)
    backlog_plugin_github_test.go:339:
        Error: Received unexpected error:
               github_issues: close issue 42 request failed: Patch "http://127.0.0.1:37119/repos/acme/widgets/issues/42":
               github: rate limited until 2026-08-17T06:37:48Z, skipping request to avoid another guaranteed failure
--- FAIL: TestGitHubIssuesPlugin_PostIssueComment_SendsExpectedBody (0.00s)
--- FAIL: TestGitHubPRsPlugin_Fetch_ParsesPRsWithReviewRequestedAndCILabels (0.00s)
--- FAIL: TestGitHubPRsPlugin_Fetch_ConcurrentCIFetchPreservesPerPRLabels (0.00s)
```

All four failures hit a **local `httptest` mock server** (`http://127.0.0.1:<port>/...`), not
the real GitHub API — yet the error is a rate-limit rejection with a real-looking future
timestamp. Root cause: `github/rate_limit.go:24` —

```go
// DefaultRateLimiter is the shared GitHub API rate limiter used by all native
// HTTP calls. It is updated automatically by rateLimitTransport on every
// response; pollers check IsLimited() before dispatching work.
var DefaultRateLimiter = &RateLimiter{}
```

is a **package-level singleton**, not per-client or per-test state.
`TestGitHubIssuesPlugin_CloseIssue_RateLimitedReturnsError` (`backlog_plugin_github_test.go:312`)
deliberately serves a 403 + `Retry-After` response from its own mock server, specifically to
verify the client surfaces a "rate limited" error — but if that test's client is wired to
(or falls back to) `DefaultRateLimiter`, the simulated hit updates the *global* singleton's
state, which every subsequently-run test in the same `go test` binary then inherits, even
though their own mock servers never returned anything rate-limit-related.

Confirmed unrelated to PR #538's diff — `git diff main...HEAD -- session/backlog_plugin_github_test.go
session/backlog_plugin_github.go` is empty, and no commit in this branch touches
`github/rate_limit.go`. `go test ./...` (`-short`, no coverage) passed cleanly locally on this
branch before the CI run, consistent with this being an ordering/coverage-instrumentation
sensitive flake (test order and/or timing shifts enough under `-coverpkg` for the global's
prior state to bleed into a different set of tests than it does locally) rather than a real
regression from this PR's changes. Filed per the blast-radius exception in
`.claude/rules/fix-flaky-tests-dont-defer.md` rather than root-caused in PR #538, since
`github/rate_limit.go`'s client-wiring is unrelated to rules/claude-settings and out of scope
for that PR.

## Fix Approach

- Confirm which test client(s) resolve to `DefaultRateLimiter` instead of a fresh
  `&RateLimiter{}` per test (likely a constructor default-arg or package-level client
  fallback in `github/client.go`/`github/http_client.go`).
- Give every test in `backlog_plugin_github_test.go` (and any other file constructing a real
  `*github.Client` against a local mock server) its own `&github.RateLimiter{}` instance
  instead of falling through to the shared `DefaultRateLimiter`, mirroring the isolation
  fix pattern already used for `TestWriteSettingsAtomic_ConcurrentWritesToSameSettingsPath_NeverProduceCorruptJSON`
  (see `server/services/hook_injector_test.go`'s doc comment) and BUG-076.
- Add a regression test that runs `TestGitHubIssuesPlugin_CloseIssue_RateLimitedReturnsError`
  immediately before `TestGitHubIssuesPlugin_CloseIssue_ReturnsResponseUpdatedAt` in the same
  `go test` invocation and asserts the second test still passes, to lock in the fix.

## Related Tasks

Found during Gate 4 (remote CI) of the `dynamic-rule-reload` backlog item's PR ship loop
(PR #538). Not fixed there — out of scope (unrelated pre-existing GitHub-plugin test
infrastructure vs. a rules-hot-reload feature). CI job re-run to confirm this is genuinely
order/timing-dependent rather than a new, deterministic failure.
