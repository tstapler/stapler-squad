package session

// Deterministic (fake clock, fake fetch) coverage for the per-PR minimum poll
// interval: scanner completions arrive every few seconds but each PR must be
// polled at most once per PollInterval, unless its worktree state changed.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/github"
)

type fakePollEnv struct {
	p     *WorktreePRPoller
	mu    sync.Mutex
	clock time.Time
	calls map[string]int
}

func newFakePollEnv(t *testing.T) *fakePollEnv {
	t.Helper()
	env := &fakePollEnv{clock: time.Unix(1_700_000_000, 0), calls: map[string]int{}}
	p := NewWorktreePRPoller(github.NewETagCache(), nil)
	p.ctx = context.Background()
	p.authState.Store(pollerAuthResult{ok: true, checkedAt: time.Now().Add(time.Hour)})
	p.now = func() time.Time {
		env.mu.Lock()
		defer env.mu.Unlock()
		return env.clock
	}
	p.fetch = func(_ context.Context, item WorktreeScanItem) {
		env.mu.Lock()
		env.calls[item.Branch]++
		env.mu.Unlock()
	}
	env.p = p
	return env
}

func (e *fakePollEnv) advance(d time.Duration) {
	e.mu.Lock()
	e.clock = e.clock.Add(d)
	e.mu.Unlock()
}

func (e *fakePollEnv) total() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, c := range e.calls {
		n += c
	}
	return n
}

func testItems(n int, fingerprint string) []WorktreeScanItem {
	items := make([]WorktreeScanItem, n)
	for i := range items {
		b := string(rune('a' + i))
		items[i] = WorktreeScanItem{RepoPath: "/repo", Branch: b, WorktreePath: "/wt/" + b, Fingerprint: fingerprint}
	}
	return items
}

func TestWorktreePRPoller_ScanDoneBurstPollsEachPROncePerInterval(t *testing.T) {
	env := newFakePollEnv(t)
	items := testItems(35, "h1")

	// One hour of scan completions every 8.5s, the cadence seen in production.
	const scanEvery = 8500 * time.Millisecond
	scans := 0
	for elapsed := time.Duration(0); elapsed < time.Hour; elapsed += scanEvery {
		env.p.pollWorktrees(items, pollTriggerScan)
		env.advance(scanEvery)
		scans++
	}

	perPRMax := int(time.Hour/env.p.config.PollInterval) + 1
	if got := env.total(); got > 35*perPRMax {
		t.Fatalf("%d scan-done events caused %d polls; want <= %d (one per PR per %s)",
			scans, got, 35*perPRMax, env.p.config.PollInterval)
	}
	t.Logf("simulated 1h: %d scan events x 35 PRs = %d polls without gating; with gating: %d",
		scans, scans*35, env.total())
}

func TestWorktreePRPoller_ScanDoneWithinIntervalSkips(t *testing.T) {
	env := newFakePollEnv(t)
	items := testItems(3, "h1")
	for i := 0; i < 5; i++ {
		env.p.pollWorktrees(items, pollTriggerScan)
		env.advance(8 * time.Second)
	}
	if got := env.total(); got != 3 {
		t.Fatalf("polls = %d, want 3 (one per PR)", got)
	}
	env.advance(env.p.config.PollInterval)
	env.p.pollWorktrees(items, pollTriggerScan)
	if got := env.total(); got != 6 {
		t.Fatalf("polls after interval elapsed = %d, want 6", got)
	}
}

func TestWorktreePRPoller_FingerprintChangePollsPromptly(t *testing.T) {
	env := newFakePollEnv(t)
	env.p.pollWorktrees(testItems(2, "h1"), pollTriggerScan)
	env.advance(2 * time.Second)

	changed := testItems(2, "h1")
	changed[0].Fingerprint = "h2" // new commit on branch "a" only
	env.p.pollWorktrees(changed, pollTriggerScan)

	if env.calls["a"] != 2 || env.calls["b"] != 1 {
		t.Fatalf("calls = %v, want a polled again (HEAD changed) and b not", env.calls)
	}
}

func TestWorktreePRPoller_TickerAlwaysPolls(t *testing.T) {
	env := newFakePollEnv(t)
	items := testItems(2, "h1")
	env.p.pollWorktrees(items, pollTriggerScan)
	env.advance(time.Second)
	env.p.pollWorktrees(items, pollTriggerTick)
	if got := env.total(); got != 4 {
		t.Fatalf("polls = %d, want 4 (ticker bypasses the gate)", got)
	}
}

func TestWorktreePRPoller_DuplicateBranchPolledOncePerPass(t *testing.T) {
	env := newFakePollEnv(t)
	items := []WorktreeScanItem{
		{RepoPath: "/repo", Branch: "main", WorktreePath: "/wt/1"},
		{RepoPath: "/repo", Branch: "main", WorktreePath: "/wt/2"},
	}
	env.p.pollWorktrees(items, pollTriggerTick)
	if got := env.total(); got != 1 {
		t.Fatalf("polls = %d, want 1 for two worktrees on the same repo+branch", got)
	}
}
