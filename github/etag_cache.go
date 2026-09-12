package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// etagCacheEntryTTL bounds how long an ETagCache entry survives without being
// re-written. The cache is per-process and keyed by every (owner, repo,
// prNumber) ever polled; Invalidate() is only called from the two webhook-
// reconciliation call sites, never on PR merge/close/session-archival/
// worktree-removal, so without a TTL the map grows unbounded over the life
// of a long-running process. A PR that hasn't been polled in a day is very
// likely closed/archived — letting its entry expire just costs the next poll
// one uncached fetch instead of a 304.
const etagCacheEntryTTL = 24 * time.Hour

// etagCacheSweepEveryNWrites triggers an opportunistic sweepExpired pass
// every N calls to set(), rather than running a dedicated background
// goroutine — set()'s caller (GetPRInfoConditional, polled continuously by
// session/pr_status_poller.go) already drives the cache's write rate, so
// piggybacking eviction on writes needs no extra lifecycle/ctx wiring.
const etagCacheSweepEveryNWrites = 64

// ETagCache stores ETags and cached PRInfo responses per (owner, repo, prNumber).
// Using conditional requests (If-None-Match) allows GitHub to return 304 Not Modified
// responses that cost zero rate-limit quota when the PR has not changed.
// sync.Map gives lock-free reads in the steady state — entries are written once on
// first PR discovery and then read on every subsequent poll tick.
type ETagCache struct {
	store      sync.Map // key: string, value: etagEntry
	writeCount atomic.Uint64
}

type etagEntry struct {
	etag        string
	prInfo      *PRInfo
	lastWriteAt time.Time
}

// NewETagCache creates a new empty ETagCache.
func NewETagCache() *ETagCache {
	return &ETagCache{}
}

func (c *ETagCache) cacheKey(ref RepoRef, prNumber int) string {
	host := ref.Host()
	if host == "" {
		host = "github.com"
	}
	return fmt.Sprintf("%s/%s/%s/%d", host, ref.Owner(), ref.Repo(), prNumber)
}

func (c *ETagCache) get(key string) (etagEntry, bool) {
	v, ok := c.store.Load(key)
	if !ok {
		return etagEntry{}, false
	}
	return v.(etagEntry), true
}

func (c *ETagCache) set(key string, e etagEntry) {
	e.lastWriteAt = time.Now()
	c.store.Store(key, e)
	if c.writeCount.Add(1)%etagCacheSweepEveryNWrites == 0 {
		c.sweepExpired(time.Now())
	}
}

// sweepExpired deletes every entry whose lastWriteAt is older than
// etagCacheEntryTTL relative to now, returning the count removed. now is a
// parameter (rather than always time.Now()) so tests can fast-forward past
// the TTL deterministically instead of waiting on real time.
func (c *ETagCache) sweepExpired(now time.Time) int {
	removed := 0
	c.store.Range(func(key, value any) bool {
		if entry, ok := value.(etagEntry); ok && now.Sub(entry.lastWriteAt) > etagCacheEntryTTL {
			c.store.Delete(key)
			removed++
		}
		return true
	})
	return removed
}

// Invalidate drops the cached entry for (ref, prNumber), safe to call
// concurrently with get/set. The next GetPRInfoConditional call for that key
// omits If-None-Match, forcing a fresh, unconditional 200 fetch instead of a
// stale 304.
func (c *ETagCache) Invalidate(ref RepoRef, prNumber int) {
	c.store.Delete(c.cacheKey(ref, prNumber))
}

// GetPRInfoConditional fetches PR info using ETag conditional requests.
// Uses native net/http instead of a gh subprocess to avoid forkExec lock contention.
// Returns (info, changed, error).
//   - changed=false means 304 Not Modified; info contains the cached value.
//   - changed=true means 200 OK; info contains freshly fetched data.
//   - Both info and changed may be zero values when an error is returned.
func GetPRInfoConditional(ctx context.Context, ref RepoRef, prNumber int, cache *ETagCache) (*PRInfo, bool, error) {
	ctx = WithGitHubCallSite(ctx, "pr.view.conditional")

	owner, repo, host := ref.Owner(), ref.Repo(), ref.Host()
	if getGHTokenForAccount(ctx, AccountRef{Host: host}) == "" {
		return nil, false, ErrNotAuthenticated
	}

	key := cache.cacheKey(ref, prNumber)

	entry, hasCached := cache.get(key)

	apiPath := fmt.Sprintf("repos/%s/%s/pulls/%d",
		url.PathEscape(owner), url.PathEscape(repo), prNumber)
	req, err := newGHRequestForHost(ctx, host, apiPath)
	if err != nil {
		return nil, false, fmt.Errorf("build conditional PR request: %w", err)
	}
	if hasCached && entry.etag != "" {
		req.Header.Set("If-None-Match", entry.etag)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("conditional PR request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		if hasCached {
			return entry.prInfo, false, nil
		}
		return nil, false, nil
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, false, fmt.Errorf("GitHub API: unauthorized (401)")
	}
	if resp.StatusCode == http.StatusForbidden {
		// Retry-After present → secondary rate limit; no Retry-After → auth/permission error.
		if resp.Header.Get("Retry-After") != "" {
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil, false, fmt.Errorf("GitHub API: secondary rate limit (403)")
		}
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil, false, fmt.Errorf("GitHub API: primary rate limit exhausted (403)")
		}
		return nil, false, fmt.Errorf("GitHub API: forbidden (403)")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, false, fmt.Errorf("GitHub API: rate limited (429)")
	}

	if resp.StatusCode != http.StatusOK {
		// Drain body so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		// Fall back to a full fetch.
		info, fetchErr := GetPRInfoCtx(ctx, ref, prNumber)
		if fetchErr != nil {
			return nil, false, fetchErr
		}
		cache.set(key, etagEntry{prInfo: info})
		return info, true, nil
	}

	// Drain body (we only need the ETag header for the conditional check).
	_, _ = io.Copy(io.Discard, resp.Body)
	newEtag := resp.Header.Get("ETag")

	// PR changed — fetch full review/CI data (requires gh CLI for reviews+statusCheckRollup).
	newInfo, err := GetPRInfoCtx(ctx, ref, prNumber)
	if err != nil {
		return nil, false, err
	}

	cache.set(key, etagEntry{etag: newEtag, prInfo: newInfo})

	return newInfo, true, nil
}
