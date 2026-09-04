package session

// worktree_pr_poller_discovery_test.go covers the same discovery-path
// ordering bug as pr_status_poller_discovery_test.go: a real
// GetPRForBranchConditional error must not be misread as a 304 "still no
// PR yet" and silently re-arm the no-PR backoff via the stale listCacheEntry.noPR flag.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/github"
)

// newDiscoveryTestRepo creates a bare-minimum local git repo with an origin
// remote pointing at a GitHub URL, so github.GetOwnerRepoFromRemote succeeds
// without any network access.
func newDiscoveryTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", dir},
		{"-C", dir, "remote", "add", "origin", "https://github.com/acme/widgets.git"},
	} {
		if out, err := safeexec.CommandContext(context.Background(), "git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

func newTestWorktreePRPoller() *WorktreePRPoller {
	p := NewWorktreePRPoller(github.NewETagCache(), nil)
	p.ctx = context.Background()
	return p
}

func TestWorktreePRPoller_FetchAndStore_DiscoveryError_NotMisreadAsNoPR(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)
	t.Setenv("GITHUB_TOKEN", "fake-token")

	repoPath := newDiscoveryTestRepo(t)
	p := newTestWorktreePRPoller()
	item := WorktreeScanItem{RepoPath: repoPath, Branch: "feature-branch", WorktreePath: repoPath}

	p.fetchAndStore(item)

	key := worktreeCacheKey(repoPath, "feature-branch")
	p.mu.Lock()
	_, backoffArmed := p.noPRPollAfter[key]
	p.mu.Unlock()
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

// TestWorktreePRPoller_FetchAndStore_StaleNoPRCache_NotReusedOnError is the
// pre-fix bug's specific manifestation in this file: a genuine error must not
// fall into the !changed branch and re-arm backoff by reading a *previous*
// successful call's stale listCacheEntry.noPR flag.
func TestWorktreePRPoller_FetchAndStore_StaleNoPRCache_NotReusedOnError(t *testing.T) {
	repoPath := newDiscoveryTestRepo(t)
	key := worktreeCacheKey(repoPath, "feature-branch")

	p := newTestWorktreePRPoller()
	// Seed a stale cache entry as if a prior successful call found no PR.
	p.listEtags.Store(key, listCacheEntry{etag: `"stale-etag"`, noPR: true})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)
	t.Setenv("GITHUB_TOKEN", "fake-token")

	item := WorktreeScanItem{RepoPath: repoPath, Branch: "feature-branch", WorktreePath: repoPath}
	p.fetchAndStore(item)

	p.mu.Lock()
	_, backoffArmed := p.noPRPollAfter[key]
	p.mu.Unlock()
	if backoffArmed {
		t.Fatal("a discovery error must not re-arm backoff from a stale cached noPR flag")
	}
}

// TestWorktreePRPoller_FetchAndStore_True304_ArmsBackoff is the positive
// control: a genuine 304 against a cached noPR=true entry must still re-arm
// backoff exactly as before the ordering fix.
func TestWorktreePRPoller_FetchAndStore_True304_ArmsBackoff(t *testing.T) {
	repoPath := newDiscoveryTestRepo(t)
	key := worktreeCacheKey(repoPath, "feature-branch")

	p := newTestWorktreePRPoller()
	p.listEtags.Store(key, listCacheEntry{etag: `"same-etag"`, noPR: true})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)
	t.Setenv("GITHUB_TOKEN", "fake-token")

	item := WorktreeScanItem{RepoPath: repoPath, Branch: "feature-branch", WorktreePath: repoPath}
	p.fetchAndStore(item)

	p.mu.Lock()
	_, backoffArmed := p.noPRPollAfter[key]
	p.mu.Unlock()
	if !backoffArmed {
		t.Fatal("a true 304 with cached noPR=true must still re-arm the no-PR backoff")
	}
}

// TestWorktreePRPoller_FetchAndStore_ErrNoPR_SetsBackoff is the positive
// control for the ErrNoPR case (changed=true, err=ErrNoPR).
func TestWorktreePRPoller_FetchAndStore_ErrNoPR_SetsBackoff(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer ts.Close()
	withGhBaseURL(t, ts)
	t.Setenv("GITHUB_TOKEN", "fake-token")

	repoPath := newDiscoveryTestRepo(t)
	p := newTestWorktreePRPoller()
	item := WorktreeScanItem{RepoPath: repoPath, Branch: "feature-branch", WorktreePath: repoPath}

	p.fetchAndStore(item)

	key := worktreeCacheKey(repoPath, "feature-branch")
	p.mu.Lock()
	_, backoffArmed := p.noPRPollAfter[key]
	p.mu.Unlock()
	if !backoffArmed {
		t.Fatal("ErrNoPR (empty PR list) must still arm the no-PR backoff")
	}
}
