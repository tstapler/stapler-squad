package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// LatestRelease is the subset of GitHub's "latest release" payload the
// update check needs.
type LatestRelease struct {
	TagName     string
	Version     string // TagName without a leading "v"
	HTMLURL     string
	PublishedAt time.Time
}

// ReleaseFetcher fetches a repo's latest release with ETag conditional
// requests: after the first 200, a 304 costs no rate-limit quota and returns
// the previously parsed release. It owns a private ETagCache so release
// URLs never share keys with PR polling.
type ReleaseFetcher struct {
	slug  string // "owner/repo"
	cache *ETagCache

	mu   sync.Mutex
	last *LatestRelease
}

// NewReleaseFetcher returns a fetcher for slug ("owner/repo")'s latest release.
func NewReleaseFetcher(slug string) *ReleaseFetcher {
	return &ReleaseFetcher{slug: slug, cache: NewETagCache()}
}

func (f *ReleaseFetcher) path() string {
	owner, repo, _ := strings.Cut(f.slug, "/")
	return fmt.Sprintf("repos/%s/%s/releases/latest", url.PathEscape(owner), url.PathEscape(repo))
}

func decodeLatestRelease(r io.Reader) (*LatestRelease, error) {
	var payload struct {
		TagName     string    `json:"tag_name"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
	}
	if err := json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode latest release: %w", err)
	}
	return &LatestRelease{
		TagName:     payload.TagName,
		Version:     strings.TrimPrefix(payload.TagName, "v"),
		HTMLURL:     payload.HTMLURL,
		PublishedAt: payload.PublishedAt,
	}, nil
}

// Latest returns the latest published release. notModified is true when
// GitHub answered 304 and the cached release was returned.
func (f *ReleaseFetcher) Latest(ctx context.Context) (rel *LatestRelease, notModified bool, err error) {
	ctx = WithGitHubCallSite(ctx, "releases.latest")
	f.mu.Lock()
	defer f.mu.Unlock()

	path := f.path()
	req, err := NewConditionalRequest(ctx, path, f.cache)
	if err != nil {
		return nil, false, fmt.Errorf("build latest-release request: %w", err)
	}
	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("latest-release request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified && f.last != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return f.last, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, classifyGHResponse(resp, "no published release", false)
	}

	rel, err = decodeLatestRelease(resp.Body)
	if err != nil {
		return nil, false, err
	}
	f.last = rel
	f.cache.set(path, etagEntry{etag: resp.Header.Get("ETag")})
	return rel, false, nil
}
