package session

import (
	"errors"
	"testing"
	"time"
)

func TestWorktreePRPoller_WebhookBurstLiftsNoPRBackoffOncePerWindow(t *testing.T) {
	env := newFakePollEnv(t)
	env.p.config.NoPRBackoff = 5 * time.Minute
	items := repoItems("/r1", 30)
	noteRepos(env.p, items, "acme", "widgets")
	backoffAll := func() {
		for _, it := range items {
			env.p.setNoPRBackoff(worktreeCacheKey(it.RepoPath, it.Branch))
		}
	}

	env.p.pollWorktrees(items, pollTriggerScan) // 30 initial polls, all find no PR
	backoffAll()

	const burst = 20 // e.g. check_run events, one per second
	for i := 0; i < burst; i++ {
		env.advance(time.Second)
		env.p.InvalidateCache("acme", "widgets", i)
		env.p.pollWorktrees(items, pollTriggerScan)
		backoffAll()
	}
	if got := env.total(); got != 30+30 {
		t.Fatalf("polls = %d, want 60 (30 initial + one backoff lift for the whole burst)", got)
	}
}

func TestWorktreePRPoller_PrunesNoPRBackoffForVanishedAndExpiredKeys(t *testing.T) {
	env := newFakePollEnv(t)
	env.p.config.NoPRBackoff = 5 * time.Minute
	items := repoItems("/r1", 3)
	env.p.pollWorktrees(items, pollTriggerScan)
	for _, it := range items {
		env.p.setNoPRBackoff(worktreeCacheKey(it.RepoPath, it.Branch))
	}

	env.p.pollWorktrees(items[:2], pollTriggerScan) // third worktree vanished
	env.p.mu.Lock()
	n := len(env.p.noPRPollAfter)
	env.p.mu.Unlock()
	if n != 2 {
		t.Fatalf("noPRPollAfter has %d entries, want 2 after one worktree vanished", n)
	}

	env.advance(6 * time.Minute) // backoffs expire
	env.p.pollWorktrees(items[:2], pollTriggerScan)
	env.p.mu.Lock()
	n = len(env.p.noPRPollAfter)
	env.p.mu.Unlock()
	if n != 0 {
		t.Fatalf("noPRPollAfter has %d entries, want 0 after expiry", n)
	}
}

func TestWorktreePRPoller_FailureCountedWhenInvalidatedMidFetch(t *testing.T) {
	env := newFakePollEnv(t)
	items := repoItems("/r1", 1)
	noteRepos(env.p, items, "acme", "widgets")
	key := worktreeCacheKey(items[0].RepoPath, items[0].Branch)

	if !env.p.claimPoll(key, "h", env.p.now(), pollTriggerScan) {
		t.Fatal("first claim must succeed")
	}
	env.p.InvalidateCache("acme", "widgets", 1) // webhook lands while the fetch is in flight
	env.p.recordResult(key, "h", errors.New("boom"))

	env.p.mu.Lock()
	failures := env.p.lastPolled[key].failures
	env.p.mu.Unlock()
	if failures != 1 {
		t.Fatalf("failures = %d, want 1 (failure must survive a mid-fetch invalidation)", failures)
	}
	if env.p.claimPoll(key, "h", env.p.now().Add(time.Second), pollTriggerScan) {
		t.Fatal("key must stay gated by its failure backoff after the invalidated failure")
	}
}
