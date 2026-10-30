package session

// Failure backoff and targeted InvalidateCache coverage for WorktreePRPoller.
// Fake clock and fake fetch throughout; the fetchAndStore mapping tests use an
// httptest GitHub stub (no real network, no sleeps).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/github"
)

func TestWorktreePRPoller_PermanentlyFailingKeyBoundedOverAnHour(t *testing.T) {
	env := newFakePollEnv(t)
	items := testItems(2, "h1")
	env.fail["a"] = true // e.g. 404 / 403 / SSO-blocked: never recovers

	const scanEvery = 8500 * time.Millisecond
	for elapsed := time.Duration(0); elapsed < time.Hour; elapsed += scanEvery {
		env.p.pollWorktrees(items, pollTriggerScan)
		env.advance(scanEvery)
	}

	// Ramp 5s,10s,20s,40s then 60s steps => ~one poll per 68s scan boundary.
	if got := env.calls["a"]; got > 60 {
		t.Fatalf("permanently failing key polled %d times in 1h; want <= 60 (one per interval after the ramp)", got)
	}
	if got := env.calls["b"]; got != 53 {
		t.Fatalf("healthy key polled %d times, want 53 (unaffected)", got)
	}
}

func TestWorktreePRPoller_SuccessResetsFailureStreak(t *testing.T) {
	env := newFakePollEnv(t)
	items := testItems(1, "h1")
	env.fail["a"] = true
	for i := 0; i < 3; i++ {
		env.p.pollWorktrees(items, pollTriggerScan)
		env.advance(70 * time.Second)
	}
	env.fail["a"] = false
	env.p.pollWorktrees(items, pollTriggerScan)

	key := worktreeCacheKey("/repo", "a")
	env.p.mu.Lock()
	got := env.p.lastPolled[key].failures
	env.p.mu.Unlock()
	if got != 0 {
		t.Fatalf("failures = %d after a success, want 0", got)
	}
}

func TestWorktreePRPoller_FetchAndStore_ErrorMapping(t *testing.T) {
	noOrigin := t.TempDir()
	if out, err := safeexec.CommandContext(context.Background(), "git", "init", "-q", noOrigin).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	tests := []struct {
		name    string
		status  int
		body    string
		noOrig  bool
		wantErr bool
	}{
		{name: "304 is success", status: http.StatusNotModified},
		{name: "empty PR list (ErrNoPR) is success", status: http.StatusOK, body: "[]"},
		{name: "404 is an error", status: http.StatusNotFound, wantErr: true},
		{name: "403 is an error", status: http.StatusForbidden, wantErr: true},
		{name: "500 is an error", status: http.StatusInternalServerError, wantErr: true},
		{name: "no origin remote is success-like", noOrig: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer ts.Close()
			withGhBaseURL(t, ts)
			t.Setenv("GITHUB_TOKEN", "fake-token")

			repoPath := noOrigin
			if !tc.noOrig {
				repoPath = newDiscoveryTestRepo(t)
			}
			p := newTestWorktreePRPoller()
			err := p.fetchAndStore(context.Background(), WorktreeScanItem{RepoPath: repoPath, Branch: "feature-branch", WorktreePath: repoPath})
			if (err != nil) != tc.wantErr {
				t.Fatalf("fetchAndStore err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func repoItems(repoPath string, n int) []WorktreeScanItem {
	items := make([]WorktreeScanItem, n)
	for i := range items {
		b := repoPath + "-b" + strconv.Itoa(i)
		items[i] = WorktreeScanItem{RepoPath: repoPath, Branch: b, WorktreePath: "/wt" + b, Fingerprint: "h"}
	}
	return items
}

func noteRepos(p *WorktreePRPoller, items []WorktreeScanItem, owner, repo string) {
	for _, it := range items {
		p.noteRepo(worktreeCacheKey(it.RepoPath, it.Branch), owner, repo)
	}
}

func TestWorktreePRPoller_InvalidateCacheOnlyRearmsMatchingRepo(t *testing.T) {
	env := newFakePollEnv(t)
	r1, r2 := repoItems("/r1", 3), repoItems("/r2", 4)
	all := append(append([]WorktreeScanItem{}, r1...), r2...)
	noteRepos(env.p, r1, "acme", "widgets")
	noteRepos(env.p, r2, "other", "thing")

	env.p.pollWorktrees(all, pollTriggerScan)
	env.advance(2 * time.Second)
	env.p.InvalidateCache("ACME", "Widgets", 7) // webhook casing differs
	env.p.pollWorktrees(all, pollTriggerScan)

	if got := env.total(); got != 7+3 {
		t.Fatalf("polls = %d, want 10 (7 initial + only the 3 acme/widgets worktrees re-armed)", got)
	}
}

func TestWorktreePRPoller_WebhookBurstOnlyRepollsInvalidatedRepo(t *testing.T) {
	env := newFakePollEnv(t)
	var all []WorktreeScanItem
	for r := 0; r < 7; r++ {
		items := repoItems("/r"+strconv.Itoa(r), 5)
		noteRepos(env.p, items, "owner", "repo"+strconv.Itoa(r))
		all = append(all, items...)
	}
	env.p.pollWorktrees(all, pollTriggerScan) // 35 initial polls

	const burst = 20
	for i := 0; i < burst; i++ {
		env.advance(time.Second) // 20s total: other repos stay inside the 60s interval
		env.p.InvalidateCache("owner", "repo0", i)
		env.p.pollWorktrees(all, pollTriggerScan)
	}
	if got := env.total(); got != 35+burst*5 {
		t.Fatalf("polls = %d, want %d (a burst on one repo re-polls only its 5 PRs per scan)", got, 35+burst*5)
	}
}

func TestWorktreePRPoller_InvalidateCacheNilETagCache(t *testing.T) {
	p := NewWorktreePRPoller(nil, nil)
	p.InvalidateCache("acme", "widgets", 1) // must not panic
}

func TestWorktreePRPoller_ConcurrentInvalidateAndPoll(t *testing.T) {
	env := newFakePollEnv(t)
	env.p.etagCache = github.NewETagCache()
	items := repoItems("/r1", 5)
	noteRepos(env.p, items, "acme", "widgets")

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				env.p.InvalidateCache("acme", "widgets", i)
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				env.advance(time.Second)
				env.p.pollWorktrees(items, pollTriggerScan)
			}
		}()
	}
	wg.Wait()
}
