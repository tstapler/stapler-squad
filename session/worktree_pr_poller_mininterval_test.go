package session

// Deterministic (fake clock, fake fetch) coverage for the per-PR minimum poll
// interval: scanner completions arrive every few seconds but each PR must be
// polled at most once per PollInterval, unless its worktree state changed.

import (
	"context"
	"errors"
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
	fail  map[string]bool // branches whose fetch returns an error
}

func newFakePollEnv(t *testing.T) *fakePollEnv {
	t.Helper()
	env := &fakePollEnv{clock: time.Unix(1_700_000_000, 0), calls: map[string]int{}, fail: map[string]bool{}}
	p := NewWorktreePRPoller(github.NewETagCache(), nil)
	p.ctx = context.Background()
	p.authState.Store(pollerAuthResult{ok: true, checkedAt: time.Now().Add(time.Hour)})
	p.now = func() time.Time {
		env.mu.Lock()
		defer env.mu.Unlock()
		return env.clock
	}
	p.fetch = func(_ context.Context, item WorktreeScanItem) error {
		env.mu.Lock()
		defer env.mu.Unlock()
		env.calls[item.Branch]++
		if env.fail[item.Branch] {
			return errors.New("transient")
		}
		return nil
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

	// Scans land every 8.5s, so each PR is re-polled on the first scan at or
	// after each 60s boundary (every 8th scan = 68s): ceil(424/8) = 53 polls.
	const wantPerPR = 53
	if got := env.total(); got != 35*wantPerPR {
		t.Fatalf("%d scan-done events caused %d polls; want exactly %d", scans, got, 35*wantPerPR)
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

func TestWorktreePRPoller_InvalidateCacheReenablesPollingOnNextScan(t *testing.T) {
	env := newFakePollEnv(t)
	items := testItems(2, "h1")
	env.p.pollWorktrees(items, pollTriggerScan)
	env.advance(2 * time.Second)
	env.p.pollWorktrees(items, pollTriggerScan)
	if got := env.total(); got != 2 {
		t.Fatalf("polls before invalidation = %d, want 2", got)
	}

	noteRepos(env.p, items, "acme", "widgets")
	env.p.InvalidateCache("acme", "widgets", 7)
	env.advance(2 * time.Second)
	env.p.pollWorktrees(items, pollTriggerScan)
	if got := env.total(); got != 4 {
		t.Fatalf("polls after InvalidateCache = %d, want 4 (re-polled on the next scan)", got)
	}
}

func TestWorktreePRPoller_FailedFetchRetriedOnNextScan(t *testing.T) {
	env := newFakePollEnv(t)
	items := testItems(2, "h1")
	env.fail["a"] = true
	env.p.pollWorktrees(items, pollTriggerScan)
	env.advance(8 * time.Second)
	env.fail["a"] = false
	env.p.pollWorktrees(items, pollTriggerScan)
	env.advance(8 * time.Second)
	env.p.pollWorktrees(items, pollTriggerScan)
	if env.calls["a"] != 2 || env.calls["b"] != 1 {
		t.Fatalf("calls = %v, want a retried once after failing (then gated) and b polled once", env.calls)
	}
}

func TestWorktreePRPoller_DuplicateKeyAnyChangedFingerprintPolls(t *testing.T) {
	env := newFakePollEnv(t)
	mk := func(fpFirst, fpSecond string) []WorktreeScanItem {
		return []WorktreeScanItem{
			{RepoPath: "/repo", Branch: "main", WorktreePath: "/wt/1", Fingerprint: fpFirst},
			{RepoPath: "/repo", Branch: "main", WorktreePath: "/wt/2", Fingerprint: fpSecond},
		}
	}
	env.p.pollWorktrees(mk("x", "y"), pollTriggerScan)
	env.advance(time.Second)
	env.p.pollWorktrees(mk("x", "y"), pollTriggerScan)
	env.p.pollWorktrees(mk("y", "x"), pollTriggerScan) // order is not stable in production
	if got := env.total(); got != 1 {
		t.Fatalf("polls with unchanged fingerprints = %d, want 1", got)
	}
	env.p.pollWorktrees(mk("x", "z"), pollTriggerScan) // only the second worktree moved
	if got := env.total(); got != 2 {
		t.Fatalf("polls after the second worktree's fingerprint changed = %d, want 2", got)
	}
}

func TestWorktreePRPoller_EmptyFingerprintGatedByInterval(t *testing.T) {
	env := newFakePollEnv(t)
	items := testItems(2, "")
	env.p.pollWorktrees(items, pollTriggerScan)
	env.advance(8 * time.Second)
	env.p.pollWorktrees(items, pollTriggerScan)
	if got := env.total(); got != 2 {
		t.Fatalf("polls = %d, want 2 (empty fingerprints are equal, so interval-gated)", got)
	}
}

func TestWorktreePRPoller_LastPolledPrunedWhenWorktreesVanish(t *testing.T) {
	env := newFakePollEnv(t)
	env.p.pollWorktrees(testItems(3, "h1"), pollTriggerScan)
	env.p.pollWorktrees(testItems(1, "h1"), pollTriggerScan)
	env.p.mu.Lock()
	n := len(env.p.lastPolled)
	env.p.mu.Unlock()
	if n != 1 {
		t.Fatalf("lastPolled has %d entries, want 1 after two worktrees vanished", n)
	}
}

func TestWorktreePRPoller_NoPRBackoffUsesInjectedClock(t *testing.T) {
	env := newFakePollEnv(t)
	env.p.config.NoPRBackoff = 5 * time.Minute
	env.p.setNoPRBackoff("k")
	want := env.p.now().Add(5 * time.Minute)
	env.p.mu.Lock()
	got := env.p.noPRPollAfter["k"]
	env.p.mu.Unlock()
	if !got.Equal(want) {
		t.Fatalf("backoff until %v, want %v from the fake clock", got, want)
	}
}
