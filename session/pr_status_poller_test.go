package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/github"
)

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
