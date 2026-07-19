package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// ErrNotAuthenticated is returned by SearchUserRepos and ListRepoIssues when no
// GitHub token is configured (GITHUB_TOKEN, GH_TOKEN, or OS keychain).
var ErrNotAuthenticated = errors.New("github token not configured")

// RepoResult is the domain return type for SearchUserRepos.
type RepoResult struct {
	Owner       string
	Repo        string
	Description string
	Private     bool
}

// IssueResult is the domain return type for ListRepoIssues.
type IssueResult struct {
	Number    int
	Title     string
	State     string
	URL       string
	Labels    []string
	CreatedAt time.Time
	UpdatedAt time.Time
	IsPR      bool
}

// ghRepoJSON matches the GitHub REST API /user/repos and /search/repositories item shape.
type ghRepoJSON struct {
	FullName string `json:"full_name"`
	Name     string `json:"name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	PushedAt    string `json:"pushed_at"`
}

// ghSearchReposResponse matches the /search/repositories envelope.
type ghSearchReposResponse struct {
	Items []ghRepoJSON `json:"items"`
}

// ghIssueListJSON matches the GitHub REST API /repos/{owner}/{repo}/issues item shape.
type ghIssueListJSON struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	State     string `json:"state"`
	HTMLURL   string `json:"html_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
	// PullRequest is non-null in the GitHub API response when the item is a PR.
	PullRequest json.RawMessage `json:"pull_request"`
}

// ghIssueSearchResponse matches the /search/issues envelope.
type ghIssueSearchResponse struct {
	Items []ghIssueListJSON `json:"items"`
}

// SearchUserRepos fetches repos accessible to the authenticated user.
// When query is empty it uses GET /user/repos (all accessible repos sorted by
// push time). When non-empty it uses GET /search/repositories.
// Returns ErrNotAuthenticated when no token is configured.
func SearchUserRepos(ctx context.Context, query string, limit int) ([]RepoResult, error) {
	if getGHToken(ctx) == "" {
		return nil, ErrNotAuthenticated
	}

	var apiPath string
	if query == "" {
		apiPath = fmt.Sprintf("user/repos?per_page=%d&sort=pushed", limit)
	} else {
		apiPath = fmt.Sprintf("search/repositories?q=%s&per_page=%d", url.QueryEscape(query), limit)
	}

	req, err := newGHRequest(ctx, apiPath)
	if err != nil {
		return nil, fmt.Errorf("build repos request: %w", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("repos request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API: unauthorized (401): %s", strings.TrimSpace(string(body)))
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
		return nil, fmt.Errorf("GitHub API: forbidden (403): %s", strings.TrimSpace(string(body)))
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
		return nil, fmt.Errorf("read repos response: %w", err)
	}

	var items []ghRepoJSON
	if query == "" {
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, fmt.Errorf("parse repos response: %w", err)
		}
	} else {
		var envelope ghSearchReposResponse
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, fmt.Errorf("parse search repos response: %w", err)
		}
		items = envelope.Items
	}

	results := make([]RepoResult, 0, len(items))
	for _, item := range items {
		results = append(results, RepoResult{
			Owner:       item.Owner.Login,
			Repo:        item.Name,
			Description: item.Description,
			Private:     item.Private,
		})
	}
	return results, nil
}

// ListRepoIssues fetches issues for a specific repo.
// When search is empty it uses GET /repos/{owner}/{repo}/issues.
// When search is non-empty it uses GET /search/issues.
// Returns ErrNotAuthenticated when no token is configured.
func ListRepoIssues(ctx context.Context, owner, repo, state, search string, limit int) ([]IssueResult, error) {
	if getGHToken(ctx) == "" {
		return nil, ErrNotAuthenticated
	}

	var apiPath string
	if search == "" {
		if state == "" {
			state = "open"
		}
		apiPath = fmt.Sprintf("repos/%s/%s/issues?state=%s&per_page=%d",
			url.PathEscape(owner), url.PathEscape(repo), url.QueryEscape(state), limit)
	} else {
		q := fmt.Sprintf("%s+repo:%s/%s+is:issue", url.QueryEscape(search), owner, repo)
		if state != "" && state != "all" {
			q += "+is:" + state
		}
		apiPath = fmt.Sprintf("search/issues?q=%s&per_page=%d", q, limit)
	}

	req, err := newGHRequest(ctx, apiPath)
	if err != nil {
		return nil, fmt.Errorf("build issues request: %w", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("issues request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API: unauthorized (401): %s", strings.TrimSpace(string(body)))
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
		return nil, fmt.Errorf("GitHub API: forbidden (403): %s", strings.TrimSpace(string(body)))
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
		return nil, fmt.Errorf("read issues response: %w", err)
	}

	var items []ghIssueListJSON
	if search == "" {
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, fmt.Errorf("parse issues response: %w", err)
		}
	} else {
		var envelope ghIssueSearchResponse
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, fmt.Errorf("parse search issues response: %w", err)
		}
		items = envelope.Items
	}

	results := make([]IssueResult, 0, len(items))
	for _, item := range items {
		labels := make([]string, len(item.Labels))
		for i, l := range item.Labels {
			labels[i] = l.Name
		}
		createdAt, _ := time.Parse(time.RFC3339, item.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339, item.UpdatedAt)
		isPR := len(item.PullRequest) > 0 && string(item.PullRequest) != "null"
		results = append(results, IssueResult{
			Number:    item.Number,
			Title:     item.Title,
			State:     item.State,
			URL:       item.HTMLURL,
			Labels:    labels,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			IsPR:      isPR,
		})
	}
	return results, nil
}
