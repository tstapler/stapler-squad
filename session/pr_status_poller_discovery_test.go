package session

// pr_status_poller_discovery_test.go covers the discovery-path ordering bug: a real
// GetPRForBranchConditional error must not be misread as a 304 "still no
// PR yet" and silently re-arm the no-PR backoff.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tstapler/stapler-squad/github"
)

// withGhBaseURL points github.GhBaseURL at ts for the duration of the test.
// Not parallel-safe: mutates a shared package global.
func withGhBaseURL(t *testing.T, ts *httptest.Server) {
	t.Helper()
	t.Cleanup(github.SetGhBaseURLForTest(ts.URL + "/"))
}

// newTestPRStatusPoller returns a poller with just enough state to call
// fetchAndUpdatePRStatus directly, without spinning up the pollLoop goroutine
// (which would require a passing auth check via Start/checkAllSessions).
func newTestPRStatusPoller() *PRStatusPoller {
	p := NewPRStatusPoller(nil)
	p.ctx = context.Background()
	return p
}

func newDiscoveryTestInstance(t *testing.T) *Instance {
	t.Helper()
	inst, err := NewInstance(InstanceOptions{
		Title:       "discovery-test",
		Path:        t.TempDir(),
		Branch:      "feature-branch",
		GitHubOwner: "acme",
		GitHubRepo:  "widgets",
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	return inst
}

func TestFetchAndUpdatePRStatus_DiscoveryError_NotMisreadAsNoPR(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)
	t.Setenv("GITHUB_TOKEN", "fake-token")

	p := newTestPRStatusPoller()
	inst := newDiscoveryTestInstance(t)

	p.fetchAndUpdatePRStatus(inst)

	p.mu.RLock()
	_, backoffArmed := p.noPRPollAfter[inst.Title]
	p.mu.RUnlock()
	if backoffArmed {
		t.Fatal("401 discovery error must not arm the no-PR backoff")
	}

	v := p.authState.Load()
	if v == nil {
		t.Fatal("expected handleFetchError to record an auth-state result for a 401")
	}
	if r := v.(pollerAuthResult); r.ok {
		t.Fatal("expected auth state to be marked failed after a 401 discovery error")
	}
}

func TestFetchAndUpdatePRStatus_DiscoveryRateLimited_NotMisreadAsNoPR(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)
	t.Setenv("GITHUB_TOKEN", "fake-token")

	p := newTestPRStatusPoller()
	inst := newDiscoveryTestInstance(t)

	p.fetchAndUpdatePRStatus(inst)

	p.mu.RLock()
	_, backoffArmed := p.noPRPollAfter[inst.Title]
	p.mu.RUnlock()
	if backoffArmed {
		t.Fatal("429 discovery error must not arm the no-PR backoff")
	}
}

func TestFetchAndUpdatePRStatus_DiscoveryParseFailure_NotMisreadAsNoPR(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)
	t.Setenv("GITHUB_TOKEN", "fake-token")

	p := newTestPRStatusPoller()
	inst := newDiscoveryTestInstance(t)

	p.fetchAndUpdatePRStatus(inst)

	p.mu.RLock()
	_, backoffArmed := p.noPRPollAfter[inst.Title]
	p.mu.RUnlock()
	if backoffArmed {
		t.Fatal("a body-parse failure must not arm the no-PR backoff")
	}
}

// TestFetchAndUpdatePRStatus_DiscoveryTrue304_ArmsBackoff is the positive
// control: a genuine 304 (changed=false, err=nil) must still re-arm the
// no-PR backoff exactly as before the ordering fix.
func TestFetchAndUpdatePRStatus_DiscoveryTrue304_ArmsBackoff(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"same-etag"`)
		w.WriteHeader(http.StatusNotModified)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)
	t.Setenv("GITHUB_TOKEN", "fake-token")

	p := newTestPRStatusPoller()
	inst := newDiscoveryTestInstance(t)

	p.fetchAndUpdatePRStatus(inst)

	p.mu.RLock()
	_, backoffArmed := p.noPRPollAfter[inst.Title]
	p.mu.RUnlock()
	if !backoffArmed {
		t.Fatal("a true 304 (unchanged, no error) must still re-arm the no-PR backoff")
	}
}

// TestFetchAndUpdatePRStatus_DiscoveryErrNoPR_AppliesNoPR is the positive
// control for the ErrNoPR case (changed=true, err=ErrNoPR): must still route
// to applyNoPR, not the generic-error branch.
func TestFetchAndUpdatePRStatus_DiscoveryErrNoPR_AppliesNoPR(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)
	t.Setenv("GITHUB_TOKEN", "fake-token")

	p := newTestPRStatusPoller()
	inst := newDiscoveryTestInstance(t)

	p.fetchAndUpdatePRStatus(inst)

	p.mu.RLock()
	_, backoffArmed := p.noPRPollAfter[inst.Title]
	p.mu.RUnlock()
	if !backoffArmed {
		t.Fatal("ErrNoPR (empty PR list) must still arm the no-PR backoff via applyNoPR")
	}
	if snap := inst.Snapshot(); snap.GitHub.GitHubPRPriority != string(github.PRPriorityNoPR) {
		t.Fatalf("expected priority %q after ErrNoPR, got %q", github.PRPriorityNoPR, snap.GitHub.GitHubPRPriority)
	}
}
