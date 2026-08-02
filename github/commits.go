package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// CommitResult is the domain return type for GetCommit.
type CommitResult struct {
	SHA     string
	HTMLURL string
	Message string
	Author  string
}

// ghCommitJSON matches the minimal fields this package needs from the GitHub
// REST API /repos/{owner}/{repo}/commits/{sha} response.
type ghCommitJSON struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Commit  struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"commit"`
}

// GetCommit fetches a single commit by SHA via the GitHub REST API (native
// net/http, same auth/error-classification mechanism as GetIssue/GetPR).
// Returns ErrNotAuthenticated when no token is configured.
func GetCommit(ctx context.Context, owner, repo, sha string) (*CommitResult, error) {
	if getGHToken(ctx) == "" {
		return nil, ErrNotAuthenticated
	}

	apiPath := fmt.Sprintf("repos/%s/%s/commits/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(sha))

	req, err := newGHRequest(ctx, apiPath)
	if err != nil {
		return nil, fmt.Errorf("build commit request: %w", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("commit request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: unauthorized (401): %s", ErrGitHubAccessDenied, strings.TrimSpace(string(body)))
	}
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("%w: commit not found (404)", ErrGitHubRefNotFound)
	}
	if resp.StatusCode == 403 {
		if resp.Header.Get("Retry-After") != "" {
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil, fmt.Errorf("GitHub API: secondary rate limit (403)")
		}
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil, fmt.Errorf("GitHub API: primary rate limit exhausted (403)")
		}
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: forbidden (403): %s", ErrGitHubAccessDenied, strings.TrimSpace(string(body)))
	}
	if resp.StatusCode == 429 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("GitHub API: rate limited (429)")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read commit response: %w", err)
	}

	var item ghCommitJSON
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, fmt.Errorf("parse commit response: %w", err)
	}

	return &CommitResult{
		SHA:     item.SHA,
		HTMLURL: item.HTMLURL,
		Message: item.Commit.Message,
		Author:  item.Commit.Author.Name,
	}, nil
}
