package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestPollerInvalidationAdapter_should_DispatchFetch_When_RepoFullNameParsesAndInstanceTracksPR
// verifies InvalidateForEvent parses "owner/repo" and forwards to
// PRStatusPoller.InvalidateAndRefresh, which dispatches an out-of-band fetch —
// same underlying behavior TestInvalidateAndRefresh_should_DispatchFetchAndReturnMatchedTrue_When_OneInstanceTracksPR
// verifies directly on the poller, exercised here through the adapter.
func TestPollerInvalidationAdapter_should_DispatchFetch_When_RepoFullNameParsesAndInstanceTracksPR(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "fake-token")

	hit := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit <- struct{}{}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)

	p := newTestPRStatusPoller()
	p.AddInstance(newInvalidateTestInstance(t, "adapter-match-test", 42))
	wp := NewWorktreePRPoller(p.ETagCache(), p)

	adapter := NewPollerInvalidationAdapter(p, wp)
	adapter.InvalidateForEvent(context.Background(), "acme/widgets", 42)

	select {
	case <-hit:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the dispatched fetch to reach the fake GitHub server")
	}
}

// TestPollerInvalidationAdapter_should_NoOp_When_RepoFullNameMalformed verifies a
// repoFullName with no "/" separator (or an empty owner/repo half) is a no-op
// rather than a panic or a call with garbage owner/repo values.
func TestPollerInvalidationAdapter_should_NoOp_When_RepoFullNameMalformed(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "fake-token")

	var reached bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)

	p := newTestPRStatusPoller()
	p.AddInstance(newInvalidateTestInstance(t, "adapter-malformed-test", 42))
	wp := NewWorktreePRPoller(p.ETagCache(), p)

	adapter := NewPollerInvalidationAdapter(p, wp)
	adapter.InvalidateForEvent(context.Background(), "malformed-no-slash", 42)

	if reached {
		t.Fatal("expected no HTTP request for a malformed repoFullName")
	}
}

// TestPollerInvalidationAdapter_should_SkipNilPollers_When_EitherPollerNotConfigured
// verifies both pollers are independently nil-safe — a real production
// possibility, since WorktreePRPoller is not constructed when no GitHub token
// is available at startup (server/server.go).
func TestPollerInvalidationAdapter_should_SkipNilPollers_When_EitherPollerNotConfigured(t *testing.T) {
	// Neither poller configured: must not panic.
	adapter := NewPollerInvalidationAdapter(nil, nil)
	adapter.InvalidateForEvent(context.Background(), "acme/widgets", 42)

	// Only the PR status poller configured (the common production shape when no
	// GitHub token is available for the worktree poller): must still dispatch.
	t.Setenv("GITHUB_TOKEN", "fake-token")
	hit := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit <- struct{}{}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)

	p := newTestPRStatusPoller()
	p.AddInstance(newInvalidateTestInstance(t, "adapter-nil-worktree-test", 42))
	adapter2 := NewPollerInvalidationAdapter(p, nil)
	adapter2.InvalidateForEvent(context.Background(), "acme/widgets", 42)

	select {
	case <-hit:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the dispatched fetch to reach the fake GitHub server")
	}
}
