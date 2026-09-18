# BUG-080: `github.DefaultRateLimiter` package-level global leaks rate-limit state across tests [SEVERITY: Medium]

**Status**: ✅ Fixed (2026-08-17)
**Discovered**: 2026-08-17 (PR #538's Gate 4 CI run)
**Fixed**: 2026-08-17 — `github/rate_limit.go`, `session/backlog_plugin_github_test.go`

## Problem Description

CI's `Test` job (PR #538, https://github.com/tstapler/stapler-squad/actions/runs/32001637582/job/95303403214)
failed deterministically (reproduced identically on 2 separate CI runs) with:

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
timestamp, and the same 4 tests failed at the same point both times.

`git diff main...HEAD` on the affected files was empty at first read, which looked like a
pure pre-existing/unrelated flake — but PR #538's branch had fallen behind `main` by 2
commits, one of which (`a6e747ef2`, "fix(github): fail fast on known rate limits instead of
retrying blind") added a pre-flight check to `rateLimitTransport.RoundTrip`
(`github/http_client.go`):

```go
func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if limited, until := DefaultRateLimiter.IsLimited(); limited {
		return nil, fmt.Errorf("github: rate limited until %s, skipping request to avoid another guaranteed failure", until.Format(time.RFC3339))
	}
	resp, err := t.next.RoundTrip(req)
	...
```

`DefaultRateLimiter` (`github/rate_limit.go`) is a **package-level singleton**:

```go
var DefaultRateLimiter = &RateLimiter{}
```

`ghHTTPClient`'s `Transport` (the sole implementation `github.HTTPClient()` returns —
`github/http_client.go`) always wraps this same singleton, with no per-client or per-test
scoping at all. Before `a6e747ef2`, a "limited" `DefaultRateLimiter` was harmless to callers
that never explicitly checked `IsLimited()` — only the pollers (`worktree_pr_poller.go`,
`pr_status_poller.go`) did. `a6e747ef2` made the singleton's state observable by *every*
`RoundTrip` call, which is correct production behavior, but exposed that
`TestGitHubIssuesPlugin_CloseIssue_RateLimitedReturnsError`
(`backlog_plugin_github_test.go:312`) — which deliberately serves a 403 + `Retry-After` from
its own mock server, specifically to verify the client surfaces a "rate limited" error —
updates the *global* singleton, which every subsequently-run test in the same `go test`
binary then inherits and fails against, even though their own mock servers never returned
anything rate-limit-related. GitHub Actions' `pull_request` trigger tests the merge of the PR
branch with `main`'s current tip, so this landed for PR #538 even though the branch's own
commits never touched `github/rate_limit.go`.

Same class of issue as BUG-076 (package-level shared state bleeding between tests), one
layer up — this time a genuine `var ... = &T{}` package singleton, not incidental shared
fixture state.

## Fix

Added `(*RateLimiter).Reset()` (`github/rate_limit.go`) to clear `rateLimitedUntil`, and
called it from `withGitHubTestServer`'s `t.Cleanup` (`session/backlog_plugin_github_test.go`)
— the one setup helper all 5 affected tests already share — so no test's simulated rate-limit
state can outlive that test.

Verified: reproduced locally by merging `origin/main` (which brought in `a6e747ef2`) and
running `go test -run "TestGitHubIssuesPlugin_CloseIssue_RateLimitedReturnsError|TestGitHubIssuesPlugin_CloseIssue_ReturnsResponseUpdatedAt|TestGitHubIssuesPlugin_PostIssueComment_SendsExpectedBody" ./session/`
— failed before the fix, passed after. Full `./session/` GitHub-plugin subset
(`-run TestGitHub`) and `./github/...` both pass under `-race`.

## Related

Found during Gate 4 (remote CI) of the `dynamic-rule-reload` backlog item's PR ship loop
(PR #538). Fixed in the same PR once the true root cause (a genuine, recent regression on
`main`, not a pre-existing unrelated flake) was confirmed — merging `main` in was needed
anyway for Gate 5 (no merge conflicts), and reproducing + fixing it there was cheaper and
more certain than deferring to a second PR against a moving `main`.
