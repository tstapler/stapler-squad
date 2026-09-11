package session

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/github"
)

// fakeGHClient counts calls per (owner, repo) pair so tests can assert whether
// checkAllSessions reached fetchAndUpdatePRStatus for a given instance, without
// shelling out to the real `gh` CLI.
type fakeGHClient struct {
	getPRInfoCalls atomic.Int64
}

func (f *fakeGHClient) CheckGHAuth() error { return nil }

func (f *fakeGHClient) GetPRForBranchConditional(_ context.Context, _, _, _, etag string) (*github.PRInfo, string, bool, error) {
	return nil, etag, false, github.ErrNoPR
}

func (f *fakeGHClient) GetPRInfoConditional(_ context.Context, _, _ string, _ int, _ *github.ETagCache) (*github.PRInfo, bool, error) {
	f.getPRInfoCalls.Add(1)
	return &github.PRInfo{State: "open"}, true, nil
}

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

// TestCheckAllSessions_SkipsPausedHibernatedStoppedInstances is the regression
// test for the gap found auditing background pollers for unnecessary work on
// suspended sessions: checkAllSessions previously re-fetched GitHub PR status
// for every registered instance every tick regardless of lifecycle state,
// unlike the analogous health-check loop (see healthCheckSkipReason). Paused,
// Hibernated, and Stopped instances must not trigger an outbound GitHub call.
func TestCheckAllSessions_SkipsPausedHibernatedStoppedInstances(t *testing.T) {
	t.Parallel()

	fake := &fakeGHClient{}
	poller := NewPRStatusPoller(nil)
	poller.ghClient = fake
	poller.ctx = context.Background()
	poller.cancel = func() {}
	poller.authState.Store(pollerAuthResult{ok: true, checkedAt: time.Now()})

	makeInst := func(title string, status Status) *Instance {
		return &Instance{
			Title:          title,
			Status:         status,
			GitHubOwner:    "tstapler",
			GitHubRepo:     "stapler-squad",
			GitHubPRNumber: 1,
		}
	}

	skipped := []*Instance{
		makeInst("paused-session", Paused),
		makeInst("hibernated-session", Hibernated),
		makeInst("stopped-session", Stopped),
	}
	active := makeInst("active-session", Active)

	poller.SetInstances(append(skipped, active))
	poller.checkAllSessions()

	if got := fake.getPRInfoCalls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 GetPRInfoConditional call (for the active instance only), got %d", got)
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
