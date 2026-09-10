package session

import (
	"testing"

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
