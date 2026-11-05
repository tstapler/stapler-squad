package session

// InvalidateCache interaction tests: failure backoff, no-PR backoff, and the
// production path that registers a worktree's repo for targeted invalidation.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWorktreePRPoller_FailingRepoWithWebhookStreamStaysBounded(t *testing.T) {
	env := newFakePollEnv(t)
	items := repoItems("/r1", 2)
	noteRepos(env.p, items, "acme", "widgets")
	env.fail[items[0].Branch] = true

	const scanEvery = 8500 * time.Millisecond
	for elapsed := time.Duration(0); elapsed < time.Hour; elapsed += scanEvery {
		env.p.InvalidateCache("acme", "widgets", 1) // a webhook before every scan
		env.p.pollWorktrees(items, pollTriggerScan)
		env.advance(scanEvery)
	}
	if got := env.calls[items[0].Branch]; got > 60 {
		t.Fatalf("failing key polled %d times in 1h under a webhook stream; want <= 60", got)
	}
}

func TestWorktreePRPoller_InvalidateCacheLiftsNoPRBackoff(t *testing.T) {
	env := newFakePollEnv(t)
	env.p.config.NoPRBackoff = 5 * time.Minute
	items := repoItems("/r1", 1)
	noteRepos(env.p, items, "acme", "widgets")
	key := worktreeCacheKey(items[0].RepoPath, items[0].Branch)

	env.p.pollWorktrees(items, pollTriggerScan)
	env.p.setNoPRBackoff(key) // as fetchAndStore does after an empty PR list
	env.advance(70 * time.Second)
	env.p.pollWorktrees(items, pollTriggerScan)
	if got := env.total(); got != 1 {
		t.Fatalf("polls during no-PR backoff = %d, want 1", got)
	}

	env.p.InvalidateCache("acme", "widgets", 9) // PR opened on that branch
	env.p.pollWorktrees(items, pollTriggerScan)
	if got := env.total(); got != 2 {
		t.Fatalf("polls after webhook = %d, want 2 (backoff lifted)", got)
	}
}

func TestWorktreePRPoller_FetchAndStoreRegistersRepoForInvalidation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)
	t.Setenv("GITHUB_TOKEN", "fake-token")

	repoPath := newDiscoveryTestRepo(t) // its origin remote points at the acme/widgets repo
	p := newTestWorktreePRPoller()
	item := WorktreeScanItem{RepoPath: repoPath, Branch: "feature-branch", WorktreePath: repoPath}
	if err := p.fetchAndStore(context.Background(), item); err != nil {
		t.Fatalf("fetchAndStore: %v", err)
	}
	key := worktreeCacheKey(repoPath, "feature-branch")
	p.mu.Lock()
	got := p.keyRepo[key]
	p.lastPolled[key] = polledState{at: time.Now(), fingerprint: "h"}
	p.lastPolled["other|b"] = polledState{at: time.Now(), fingerprint: "h"}
	p.keyRepo["other|b"] = "x/y"
	p.mu.Unlock()
	if got != "acme/widgets" {
		t.Fatalf("keyRepo[%q] = %q, want acme/widgets (production must register the repo)", key, got)
	}

	p.InvalidateCache("acme", "widgets", 1)
	p.mu.Lock()
	_, rearmed := p.lastPolled[key]
	_, otherKept := p.lastPolled["other|b"]
	p.mu.Unlock()
	if rearmed || !otherKept {
		t.Fatalf("rearmed=%v otherKept=%v, want only the acme/widgets key re-armed", rearmed, otherKept)
	}
}
