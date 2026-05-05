package github

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/executor/safeexec"
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
// Returns (info, changed, error).
//   - changed=false means 304 Not Modified; info contains the cached value.
//   - changed=true means 200 OK; info contains freshly fetched data.
//   - Both info and changed may be zero values when an error is returned.
func GetPRInfoConditional(ctx context.Context, owner, repo string, prNumber int, cache *ETagCache) (*PRInfo, bool, error) {
	key := cache.cacheKey(owner, repo, prNumber)

	cache.mu.RLock()
	entry, hasCached := cache.store[key]
	cache.mu.RUnlock()

	// Lightweight REST request to check if PR has changed via ETag
	apiPath := fmt.Sprintf("repos/%s/%s/pulls/%d", owner, repo, prNumber)
	args := []string{"api", apiPath, "--include"}
	if hasCached && entry.etag != "" {
		args = append(args, "--header", fmt.Sprintf("If-None-Match: %s", entry.etag))
	}

	cmd := safeexec.CommandContext(ctx, "gh", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			// gh may exit non-zero for 304; treat as not-modified when we have cache
			if strings.Contains(stderr, "304") && hasCached {
				return entry.prInfo, false, nil
			}
			return nil, false, fmt.Errorf("gh api failed: %s", stderr)
		}
		return nil, false, fmt.Errorf("gh api failed: %w", err)
	}

	statusCode, newEtag, _, parseErr := parseGHAPIIncludeOutput(string(output))
	if parseErr != nil {
		// Parsing headers failed; fall through to a full fetch
		info, fetchErr := GetPRInfoCtx(ctx, owner, repo, prNumber)
		if fetchErr != nil {
			return nil, false, fetchErr
		}
		// Update cache so future polls can attempt conditional requests.
		cache.mu.Lock()
		cache.store[key] = etagEntry{prInfo: info}
		cache.mu.Unlock()
		return info, true, nil
	}

	// 304 Not Modified - return cached entry
	if statusCode == 304 {
		if hasCached {
			return entry.prInfo, false, nil
		}
		return nil, false, nil
	}

	// 200 OK - PR changed; fetch full review/CI data via GetPRInfoCtx
	newInfo, err := GetPRInfoCtx(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, false, err
	}

	// Update cache with new ETag and freshly fetched data
	cache.mu.Lock()
	cache.store[key] = etagEntry{etag: newEtag, prInfo: newInfo}
	cache.mu.Unlock()

	return newInfo, true, nil
}

// parseGHAPIIncludeOutput parses the output of `gh api --include`.
// gh --include outputs: "HTTP/x.x <code> <reason>\r\n<headers>\r\n\r\n<body>"
// Returns (statusCode, etag, body, error).
func parseGHAPIIncludeOutput(rawOutput string) (statusCode int, etag, body string, err error) {
	scanner := bufio.NewScanner(strings.NewReader(rawOutput))

	if !scanner.Scan() {
		return 0, "", "", fmt.Errorf("empty response from gh api")
	}
	statusLine := strings.TrimRight(scanner.Text(), "\r")
	parts := strings.Fields(statusLine)
	if len(parts) < 2 {
		return 0, "", "", fmt.Errorf("invalid status line: %q", statusLine)
	}
	code, convErr := strconv.Atoi(parts[1])
	if convErr != nil {
		return 0, "", "", fmt.Errorf("failed to parse status code from %q", statusLine)
	}
	statusCode = code

	// Parse headers until blank line
	var bodyLines []string
	inBody := false
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if inBody {
			bodyLines = append(bodyLines, line)
			continue
		}
		if line == "" {
			inBody = true
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "etag:") {
			etag = strings.TrimSpace(line[5:])
		}
	}
	body = strings.Join(bodyLines, "\n")
	return statusCode, etag, body, nil
}
