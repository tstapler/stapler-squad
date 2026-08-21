package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrNotAuthenticated is returned by SearchUserRepos and ListRepoIssues when no
// GitHub token is configured (GITHUB_TOKEN, GH_TOKEN, or OS keychain).
var ErrNotAuthenticated = errors.New("github token not configured")

// ErrGitHubRefNotFound is a definitive 404: the referenced PR/issue/commit
// doesn't exist, or exists but is invisible to the configured token (GitHub
// disguises "exists, no access" as "not found" for security).
var ErrGitHubRefNotFound = errors.New("github: reference not found")

// ErrGitHubAccessDenied is 401, or 403 with no rate-limit signal — retrying
// with the same credentials will not change the outcome.
var ErrGitHubAccessDenied = errors.New("github: access denied")

// RepoResult is the domain return type for SearchUserRepos.
type RepoResult struct {
	Owner       string
	Repo        string
	Description string
	Private     bool
}

// PRResult is the domain return type for GetPR — a lean existence check, not
// the richer PRInfo returned by GetPRInfoCtx (which shells out to `gh`).
type PRResult struct {
	Number  int
	Title   string
	State   string
	HTMLURL string
}

// ghPRJSON matches the minimal fields this package needs from the GitHub REST
// API /repos/{owner}/{repo}/pulls/{number} response.
type ghPRJSON struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
}

// IssueResult is the domain return type for ListRepoIssues and GetIssue.
type IssueResult struct {
	Number    int
	Title     string
	Body      string
	Author    string
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
// The GitHub API includes the full body on both the list and single-issue
// endpoints, so this shape is also reused (embedded) by ghIssueDetailJSON.
type ghIssueListJSON struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	HTMLURL   string `json:"html_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
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
// account.Host "" means github.com; account.Username, when non-empty, selects
// among multiple connected accounts for that host (see getGHTokenForAccount).
// Returns ErrNotAuthenticated when no token is configured.
func SearchUserRepos(ctx context.Context, account AccountRef, query string, limit int) ([]RepoResult, error) {
	token := getGHTokenForAccount(ctx, account)
	if token == "" {
		return nil, ErrNotAuthenticated
	}

	var apiPath string
	if query == "" {
		apiPath = fmt.Sprintf("user/repos?per_page=%d&sort=pushed", limit)
	} else {
		apiPath = fmt.Sprintf("search/repositories?q=%s&per_page=%d", url.QueryEscape(query), limit)
	}

	req, err := newGHRequestForHostWithToken(ctx, account.Host, apiPath, token)
	if err != nil {
		return nil, fmt.Errorf("build repos request: %w", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("repos request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyGHResponse(resp, "", false)
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
// account.Host "" means github.com; account.Username, when non-empty, selects
// among multiple connected accounts for that host (see getGHTokenForAccount).
// Returns ErrNotAuthenticated when no token is configured.
func ListRepoIssues(ctx context.Context, account AccountRef, repo RepoRef, state, search string, limit int) ([]IssueResult, error) {
	token := getGHTokenForAccount(ctx, account)
	if token == "" {
		return nil, ErrNotAuthenticated
	}

	var apiPath string
	if search == "" {
		if state == "" {
			state = "open"
		}
		apiPath = fmt.Sprintf("repos/%s/%s/issues?state=%s&per_page=%d",
			url.PathEscape(repo.Owner()), url.PathEscape(repo.Repo()), url.QueryEscape(state), limit)
	} else {
		q := fmt.Sprintf("%s+repo:%s/%s+is:issue", url.QueryEscape(search), repo.Owner(), repo.Repo())
		if state != "" && state != "all" {
			q += "+is:" + state
		}
		apiPath = fmt.Sprintf("search/issues?q=%s&per_page=%d", q, limit)
	}

	req, err := newGHRequestForHostWithToken(ctx, account.Host, apiPath, token)
	if err != nil {
		return nil, fmt.Errorf("build issues request: %w", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("issues request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyGHResponse(resp, "", false)
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
			Body:      item.Body,
			Author:    item.User.Login,
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

// GetIssue fetches a single issue (including its body) by number.
// account.Host "" means github.com; account.Username, when non-empty, selects
// among multiple connected accounts for that host (see getGHTokenForAccount).
// Returns ErrNotAuthenticated when no token is configured.
func GetIssue(ctx context.Context, account AccountRef, repo RepoRef, number int) (*IssueResult, error) {
	token := getGHTokenForAccount(ctx, account)
	if token == "" {
		return nil, ErrNotAuthenticated
	}

	apiPath := fmt.Sprintf("repos/%s/%s/issues/%d", url.PathEscape(repo.Owner()), url.PathEscape(repo.Repo()), number)

	req, err := newGHRequestForHostWithToken(ctx, account.Host, apiPath, token)
	if err != nil {
		return nil, fmt.Errorf("build issue request: %w", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("issue request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyGHResponse(resp, "issue not found (404)", true)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read issue response: %w", err)
	}

	var item ghIssueListJSON
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, fmt.Errorf("parse issue response: %w", err)
	}

	labels := make([]string, len(item.Labels))
	for i, l := range item.Labels {
		labels[i] = l.Name
	}
	createdAt, _ := time.Parse(time.RFC3339, item.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339, item.UpdatedAt)
	isPR := len(item.PullRequest) > 0 && string(item.PullRequest) != "null"

	return &IssueResult{
		Number:    item.Number,
		Title:     item.Title,
		Body:      item.Body,
		Author:    item.User.Login,
		State:     item.State,
		URL:       item.HTMLURL,
		Labels:    labels,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		IsPR:      isPR,
	}, nil
}

// GetPR fetches a single pull request by number via the GitHub REST API
// (native net/http, not the `gh` CLI subprocess GetPRInfoCtx uses) — a lean
// existence check sharing GetIssue's auth mechanism and error classification.
// account.Host "" means github.com; account.Username, when non-empty, selects
// among multiple connected accounts for that host (see getGHTokenForAccount).
// Returns ErrNotAuthenticated when no token is configured.
func GetPR(ctx context.Context, account AccountRef, repo RepoRef, number int) (*PRResult, error) {
	token := getGHTokenForAccount(ctx, account)
	if token == "" {
		return nil, ErrNotAuthenticated
	}

	apiPath := fmt.Sprintf("repos/%s/%s/pulls/%d", url.PathEscape(repo.Owner()), url.PathEscape(repo.Repo()), number)

	req, err := newGHRequestForHostWithToken(ctx, account.Host, apiPath, token)
	if err != nil {
		return nil, fmt.Errorf("build PR request: %w", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PR request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyGHResponse(resp, "PR not found (404)", true)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read PR response: %w", err)
	}

	var item ghPRJSON
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, fmt.Errorf("parse PR response: %w", err)
	}

	return &PRResult{
		Number:  item.Number,
		Title:   item.Title,
		State:   item.State,
		HTMLURL: item.HTMLURL,
	}, nil
}
