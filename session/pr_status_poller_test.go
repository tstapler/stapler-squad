package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/github"
)

// admissionControlFlagName mirrors github/http_client.go's unexported
// githubPriorityAdmissionFlagName constant. github's doc comment on that
// constant explains why the literal is duplicated (github cannot import
// server/services, which registers the same flag, without a cycle) rather
// than shared via an exported symbol; this test needs the same literal to
// flip the flag from outside the github package.
const admissionControlFlagName = "github:priority-admission-control"

// TestApplyPRUpdate_FiresOnUpdated_WhenCheckConclusionChangesWithoutPriorityChange is the
// regression test for Task 3.2.1a's changed-only-publish fix: onUpdated must fire when
// only GitHubCheckConclusion changes (no priority-boundary crossing), and must NOT fire
// when neither priority nor conclusion changed.
func TestApplyPRUpdate_FiresOnUpdated_WhenCheckConclusionChangesWithoutPriorityChange(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "ci-conclusion-test"}
	// Seed a "blocking" priority with a "pending" CI conclusion so a later "failure"
	// conclusion crosses no priority boundary (both are priority "blocking").
	inst.UpdatePRStatus(PRStatusUpdate{State: "open", Priority: "blocking", CheckConclusion: "pending"})

	poller := NewPRStatusPoller(nil)
	fired := 0
	poller.SetOnUpdated(func(*Instance) { fired++ })

	// Conclusion-only change: "pending" -> "failure", priority stays "blocking".
	poller.applyPRUpdate(inst, &github.PRInfo{
		State:                 "open",
		CheckConclusion:       "failure",
		ApprovedCount:         0,
		ChangesRequestedCount: 1, // forces DerivePRPriority to "blocking" again, same as before
		IsDraft:               false,
	})
	if fired != 1 {
		t.Fatalf("expected onUpdated to fire once for a conclusion-only change, fired=%d", fired)
	}
	if inst.Snapshot().GitHub.GitHubCheckConclusion != "failure" {
		t.Errorf("expected GitHubCheckConclusion to be updated to %q, got %q", "failure", inst.Snapshot().GitHub.GitHubCheckConclusion)
	}

	// No change at all: same conclusion, same priority-relevant inputs -> no event.
	poller.applyPRUpdate(inst, &github.PRInfo{
		State:                 "open",
		CheckConclusion:       "failure",
		ApprovedCount:         0,
		ChangesRequestedCount: 1,
		IsDraft:               false,
	})
	if fired != 1 {
		t.Fatalf("expected onUpdated NOT to fire when neither priority nor conclusion changed, fired=%d", fired)
	}
}

// TestApplyPRUpdate_should_ThreadChecksReviewsMergeable_When_PRInfoPopulated verifies
// Story 1.2.2's plumbing: applyPRUpdate must carry PRInfo's itemized Checks/Reviews and
// its Mergeable string through PRStatusUpdate onto the Instance unchanged.
func TestApplyPRUpdate_should_ThreadChecksReviewsMergeable_When_PRInfoPopulated(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "checks-reviews-mergeable-test"}
	poller := NewPRStatusPoller(nil)

	checks := []github.CheckItem{
		{Name: "build", Context: "ci/build", State: "SUCCESS", Status: "completed", Conclusion: "success"},
		{Name: "lint", Context: "ci/lint", State: "FAILURE", Status: "completed", Conclusion: "failure"},
	}
	reviews := []github.ReviewItem{
		{Author: "alice", State: "APPROVED", Body: "lgtm"},
		{Author: "bob", State: "CHANGES_REQUESTED", Body: "please fix x"},
	}

	poller.applyPRUpdate(inst, &github.PRInfo{
		State:                 "open",
		CheckConclusion:       "failure",
		Mergeable:             "conflicting",
		ApprovedCount:         1,
		ChangesRequestedCount: 1,
		IsDraft:               false,
		Checks:                checks,
		Reviews:               reviews,
	})

	snap := inst.Snapshot()
	if snap.GitHub.GitHubMergeable != "conflicting" {
		t.Errorf("expected GitHubMergeable %q, got %q", "conflicting", snap.GitHub.GitHubMergeable)
	}
	if len(snap.GitHub.GitHubChecks) != len(checks) {
		t.Fatalf("expected %d checks, got %d", len(checks), len(snap.GitHub.GitHubChecks))
	}
	for i, c := range checks {
		if snap.GitHub.GitHubChecks[i] != c {
			t.Errorf("check[%d]: expected %+v, got %+v", i, c, snap.GitHub.GitHubChecks[i])
		}
	}
	if len(snap.GitHub.GitHubReviewFeedback) != len(reviews) {
		t.Fatalf("expected %d reviews, got %d", len(reviews), len(snap.GitHub.GitHubReviewFeedback))
	}
	for i, r := range reviews {
		if snap.GitHub.GitHubReviewFeedback[i] != r {
			t.Errorf("review[%d]: expected %+v, got %+v", i, r, snap.GitHub.GitHubReviewFeedback[i])
		}
	}
}

// TestPRStatusPoller_ETagCache_ReturnsSharedNonNilInstance is the regression
// test for ADR-022's original intent: WorktreePRPoller must reuse
// PRStatusPoller's *github.ETagCache (via this getter) rather than each
// poller constructing its own, which would double GitHub API call volume for
// repos both pollers hit.
func TestPRStatusPoller_ETagCache_ReturnsSharedNonNilInstance(t *testing.T) {
	t.Parallel()
	poller := NewPRStatusPoller(nil)

	cache := poller.ETagCache()
	if cache == nil {
		t.Fatal("ETagCache() = nil, want a non-nil *github.ETagCache")
	}
	if poller.ETagCache() != cache {
		t.Error("ETagCache() returned a different instance on a repeated call, want the same shared instance every time")
	}
}

// newInvalidateTestInstance builds an *Instance pre-populated with a known PR
// so InvalidateAndRefresh's (owner, repo, prNumber) matching can be exercised
// without going through PR auto-discovery.
func newInvalidateTestInstance(t *testing.T, title string, prNumber int) *Instance {
	t.Helper()
	inst, err := NewInstance(InstanceOptions{
		Title:          title,
		Path:           t.TempDir(),
		Branch:         "feature-branch",
		GitHubOwner:    "acme",
		GitHubRepo:     "widgets",
		GitHubPRNumber: prNumber,
	})
	if err != nil {
		t.Fatalf("NewInstance(%s): %v", title, err)
	}
	return inst
}

// TestInvalidateAndRefresh_should_DispatchFetchAndReturnMatchedTrue_When_OneInstanceTracksPR
// is Task 5.1.2c's matched case: a single tracked instance matching the
// invalidated (owner, repo, prNumber) triple gets fetchAndUpdatePRStatus
// dispatched out of band.
func TestInvalidateAndRefresh_should_DispatchFetchAndReturnMatchedTrue_When_OneInstanceTracksPR(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "fake-token")

	hit := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit <- struct{}{}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)

	p := newTestPRStatusPoller()
	p.AddInstance(newInvalidateTestInstance(t, "invalidate-match-test", 42))

	matched := p.InvalidateAndRefresh(context.Background(), "acme", "widgets", 42)
	if !matched {
		t.Fatal("InvalidateAndRefresh() = false, want true for a tracked instance")
	}

	select {
	case <-hit:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the dispatched fetchAndUpdatePRStatus to reach the fake GitHub server")
	}
}

// TestInvalidateAndRefresh_should_ReturnMatchedFalse_When_NoTrackedInstanceMatches
// is Task 5.1.2c's unmatched case (name matches validation.md's REQ-4 test
// case): no dispatch happens when no tracked instance matches, and the call
// is a documented no-op rather than an error.
func TestInvalidateAndRefresh_should_ReturnMatchedFalse_When_NoTrackedInstanceMatches(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "fake-token")

	var reached bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)

	p := newTestPRStatusPoller()
	// Tracks a different PR number, so it must not match.
	p.AddInstance(newInvalidateTestInstance(t, "invalidate-no-match-test", 99))

	matched := p.InvalidateAndRefresh(context.Background(), "acme", "widgets", 42)
	if matched {
		t.Fatal("InvalidateAndRefresh() = true, want false when no tracked instance matches")
	}
	// No matching instance means no goroutine is ever spawned (the loop's
	// continue prevents it), so this read races nothing — there is nothing
	// async left in flight to check.
	if reached {
		t.Fatal("expected no HTTP request when no tracked instance matches")
	}
}

// TestInvalidateAndRefresh_should_DispatchFetchForEachInstance_When_TwoInstancesTrackSamePR
// is Task 5.1.2c's plural-match case: research/architecture.md §3 notes two
// Instances (e.g. a worktree session and its parent) can track the same PR,
// and both must be refreshed, not just the first match.
func TestInvalidateAndRefresh_should_DispatchFetchForEachInstance_When_TwoInstancesTrackSamePR(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "fake-token")

	hits := make(chan struct{}, 2)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits <- struct{}{}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)

	p := newTestPRStatusPoller()
	p.AddInstance(newInvalidateTestInstance(t, "session-a", 42))
	p.AddInstance(newInvalidateTestInstance(t, "session-b", 42))

	matched := p.InvalidateAndRefresh(context.Background(), "acme", "widgets", 42)
	if !matched {
		t.Fatal("InvalidateAndRefresh() = false, want true when two instances track the PR")
	}

	for i := 0; i < 2; i++ {
		select {
		case <-hits:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for hit %d/2 — expected fetchAndUpdatePRStatus dispatched for both matching instances", i+1)
		}
	}
}

// TestFetchAndUpdatePRStatus_AdmissionControlRejection_SkipsCleanly is Task
// 3.3.1a / Story 3.3.1's integration test: with
// github:priority-admission-control on and the "core" resource's headroom
// below backgroundHeadroomPercent (Epic 3.2's AdmitOrigin, github/rate_limit.go),
// a poller tick's GetPRInfoConditional call is rejected by
// rateLimitTransport.RoundTrip (github/http_client.go) before the request ever
// reaches GitHub. fetchAndUpdatePRStatus needs no new code to handle this: the
// rejection surfaces as a plain error from GetPRInfoConditional, which the
// existing `if p.handleFetchError(err) { ... }; log.Warn(...); return` block
// (session/pr_status_poller.go) already handles like any other fetch failure —
// the tick is skipped cleanly with no PR-status mutation and no onUpdated
// callback.
//
// Deviation from the plan's literal AC wording: the rejection error text
// ("github: admission control rejected request: ...") contains none of
// handleFetchError's substring matches ("rate limit", "429", "401",
// "Unauthorized"), so handleFetchError itself returns false here and the code
// falls through to the generic log.Warn+return branch rather than
// handleFetchError's own `return true` line — unlike a real GitHub-side 403/429,
// which handleFetchError does special-case. Both branches still return
// immediately without touching PR status, so the observable behavior this
// story cares about (a clean skip) holds either way; this is confirmed below
// alongside a direct assertion on handleFetchError's actual return value so
// the distinction is explicit rather than assumed.
func TestFetchAndUpdatePRStatus_AdmissionControlRejection_SkipsCleanly(t *testing.T) {
	// Deliberately not github.ResetRateLimiterForTest(t): that helper swaps the
	// DefaultRateLimiter *pointer* itself, which races (under go test -race)
	// against any still-in-flight goroutine elsewhere in the binary reading the
	// global var — e.g. a prior TestInvalidateAndRefresh_* test's fire-and-forget
	// fetchAndUpdatePRStatus dispatch, which that test only waits on to the
	// extent of observing the HTTP hit, not full goroutine completion.
	// RateLimiter.Reset() only clears rateLimitedUntil under its own mutex
	// (matching IsLimited()'s locking), so it can't race with a concurrent
	// reader/writer the way a bare pointer reassignment can, at the cost of not
	// resetting resource quotas — harmless here since the priority-admission
	// flag (restored to its prior value below) gates every path that reads them.
	github.DefaultRateLimiter.Reset()

	cfg := config.LoadConfig()
	prevFlag := cfg.GetFeatureFlag(admissionControlFlagName)
	if err := cfg.SetFeatureFlag(admissionControlFlagName, true); err != nil {
		t.Fatalf("SetFeatureFlag(%q, true) failed: %v", admissionControlFlagName, err)
	}
	t.Cleanup(func() {
		if err := config.LoadConfig().SetFeatureFlag(admissionControlFlagName, prevFlag); err != nil {
			t.Errorf("cleanup: failed to restore %q to %v: %v", admissionControlFlagName, prevFlag, err)
		}
	})

	// Prime the shared RateLimiter with a below-headroom "core" quota (400/5000 =
	// 8%, under backgroundHeadroomPercent's 10% reserve) so AdmitOrigin rejects a
	// background-origin call targeting "core" — which a PR-info request is
	// classified as by github.ResourceForRequest.
	quotaHeaders := make(http.Header)
	quotaHeaders.Set("X-RateLimit-Resource", "core")
	quotaHeaders.Set("X-RateLimit-Remaining", "400")
	quotaHeaders.Set("X-RateLimit-Limit", "5000")
	github.DefaultRateLimiter.Update(&http.Response{StatusCode: http.StatusOK, Header: quotaHeaders}) //nolint:bodyclose // synthetic response, no real body

	var reached bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)
	t.Setenv("GITHUB_TOKEN", "fake-token")

	p := newTestPRStatusPoller()
	inst := newInvalidateTestInstance(t, "admission-rejection-test", 42)

	var updated int
	p.SetOnUpdated(func(*Instance) { updated++ })

	// Sanity-check the rejection actually reaches GetPRInfoConditional as a plain
	// error (not a panic, not a false "unchanged" result) before exercising the
	// full fetchAndUpdatePRStatus path below.
	_, changed, err := github.GetPRInfoConditional(context.Background(), "acme", "widgets", 42, p.ETagCache())
	if err == nil {
		t.Fatal("GetPRInfoConditional() error = nil, want an admission-control rejection error")
	}
	if changed {
		t.Fatal("GetPRInfoConditional() changed = true, want false on a rejected call")
	}
	if reached {
		t.Fatal("expected the request to be rejected by AdmitOrigin before reaching the fake GitHub server")
	}
	if p.handleFetchError(err) {
		t.Fatal("handleFetchError(err) = true for an admission-control rejection, want false — its message matches none of handleFetchError's rate-limit/auth substrings (see deviation note in this test's doc comment)")
	}

	// Full path: fetchAndUpdatePRStatus must still skip cleanly via the
	// generic log.Warn+return fallthrough, with no PR-status mutation.
	p.fetchAndUpdatePRStatus(context.Background(), inst)

	if reached {
		t.Fatal("expected fetchAndUpdatePRStatus's request to be rejected by AdmitOrigin before reaching the fake GitHub server")
	}
	if updated != 0 {
		t.Fatalf("expected onUpdated not to fire when the fetch was rejected by admission control, fired=%d", updated)
	}
	if snap := inst.Snapshot(); snap.GitHub.GitHubPRPriority != "" {
		t.Fatalf("expected PR priority to remain unset after an admission-control rejection, got %q", snap.GitHub.GitHubPRPriority)
	}
}
