package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
)

// ETagCache stores ETags and cached PRInfo responses per (owner, repo, prNumber).
// Using conditional requests (If-None-Match) allows GitHub to return 304 Not Modified
// responses that cost zero rate-limit quota when the PR has not changed.
type ETagCache struct {
	mu    sync.RWMutex
	store map[string]etagEntry
}

type etagEntry struct {
	etag   string
	prInfo *PRInfo
}

// NewETagCache creates a new empty ETagCache.
func NewETagCache() *ETagCache {
	return &ETagCache{
		store: make(map[string]etagEntry),
	}
}

func (c *ETagCache) cacheKey(owner, repo string, prNumber int) string {
	return fmt.Sprintf("%s/%s/%d", owner, repo, prNumber)
}

// GetPRInfoConditional fetches PR info using ETag conditional requests.
// Uses native net/http instead of a gh subprocess to avoid forkExec lock contention.
// Returns (info, changed, error).
//   - changed=false means 304 Not Modified; info contains the cached value.
//   - changed=true means 200 OK; info contains freshly fetched data.
//   - Both info and changed may be zero values when an error is returned.
func GetPRInfoConditional(ctx context.Context, owner, repo string, prNumber int, cache *ETagCache) (*PRInfo, bool, error) {
	key := cache.cacheKey(owner, repo, prNumber)

	cache.mu.RLock()
	entry, hasCached := cache.store[key]
	cache.mu.RUnlock()

	apiPath := fmt.Sprintf("repos/%s/%s/pulls/%d",
		url.PathEscape(owner), url.PathEscape(repo), prNumber)
	req, err := newGHRequest(ctx, apiPath)
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

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, false, fmt.Errorf("GitHub API: auth error (%d)", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		// Drain body so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		// Fall back to a full fetch.
		info, fetchErr := GetPRInfoCtx(ctx, owner, repo, prNumber)
		if fetchErr != nil {
			return nil, false, fetchErr
		}
		cache.mu.Lock()
		cache.store[key] = etagEntry{prInfo: info}
		cache.mu.Unlock()
		return info, true, nil
	}

	// Drain body (we only need the ETag header for the conditional check).
	_, _ = io.Copy(io.Discard, resp.Body)
	newEtag := resp.Header.Get("ETag")

	// PR changed — fetch full review/CI data (requires gh CLI for reviews+statusCheckRollup).
	newInfo, err := GetPRInfoCtx(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, false, err
	}

	cache.mu.Lock()
	cache.store[key] = etagEntry{etag: newEtag, prInfo: newInfo}
	cache.mu.Unlock()

	return newInfo, true, nil
}
