package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const githubPRsPerPage = 50

// githubPRPluginConfig holds the decoded config for the GitHub PRs plugin.
type githubPRPluginConfig struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Token string `json:"token"`
}

// githubPR is the subset of fields from the GitHub Pulls API response.
type githubPR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	UpdatedAt string `json:"updated_at"`
	HTMLURL   string `json:"html_url"`
	State     string `json:"state"`
	Head      struct {
		SHA string `json:"sha"`
	} `json:"head"`
	RequestedReviewers []struct {
		Login string `json:"login"`
	} `json:"requested_reviewers"`
}

// githubCheckRun is a subset of fields from the GitHub Check Runs API.
type githubCheckRun struct {
	Conclusion string `json:"conclusion"`
}

// GitHubPRsPlugin fetches open pull requests from a GitHub repository.
type GitHubPRsPlugin struct{}

// NewGitHubPRsPlugin returns a new GitHubPRsPlugin.
func NewGitHubPRsPlugin() *GitHubPRsPlugin {
	return &GitHubPRsPlugin{}
}

// PluginID returns the unique identifier for this plugin.
func (g *GitHubPRsPlugin) PluginID() string {
	return "github_prs"
}

// Fetch retrieves open pull requests. Cursor is unused (full refresh each time).
// Returns empty list when token is absent.
func (g *GitHubPRsPlugin) Fetch(ctx context.Context, config PluginConfig, cursor string) ([]ExternalItem, string, error) {
	var cfg githubPRPluginConfig
	if config.Raw != "" {
		if err := json.Unmarshal([]byte(config.Raw), &cfg); err != nil {
			return nil, cursor, fmt.Errorf("github_prs: parse config: %w", err)
		}
	}

	if cfg.Token == "" {
		return nil, cursor, nil
	}
	if cfg.Owner == "" || cfg.Repo == "" {
		return nil, cursor, fmt.Errorf("github_prs: owner and repo are required in config")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=open&per_page=%d", cfg.Owner, cfg.Repo, githubPRsPerPage)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, cursor, fmt.Errorf("github_prs: build request: %w", err)
	}
	req.Header.Set("Authorization", "token "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, cursor, fmt.Errorf("github_prs: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0") {
		return nil, cursor, fmt.Errorf("github_prs: rate limited (status %d)", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, cursor, fmt.Errorf("github_prs: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var prs []githubPR
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		return nil, cursor, fmt.Errorf("github_prs: decode response: %w", err)
	}

	items := make([]ExternalItem, 0, len(prs))
	for _, pr := range prs {
		labels := g.computeLabels(ctx, cfg, pr)
		items = append(items, ExternalItem{
			ExternalID:  strconv.Itoa(pr.Number),
			Title:       pr.Title,
			Description: pr.Body,
			Labels:      labels,
			Priority:    3,
			URL:         pr.HTMLURL,
		})
	}

	return items, cursor, nil
}

// computeLabels derives state-based labels for a PR.
func (g *GitHubPRsPlugin) computeLabels(ctx context.Context, cfg githubPRPluginConfig, pr githubPR) []string {
	var labels []string

	if len(pr.RequestedReviewers) > 0 {
		labels = append(labels, "pr:review-requested")
	}

	// Check CI status via check runs API (best-effort; errors are silent).
	if pr.Head.SHA != "" {
		ciLabel := g.fetchCILabel(ctx, cfg, pr.Head.SHA)
		if ciLabel != "" {
			labels = append(labels, ciLabel)
		}
	}

	return labels
}

// fetchCILabel calls the check runs API and returns "pr:ci-failing" when any check has failed.
func (g *GitHubPRsPlugin) fetchCILabel(ctx context.Context, cfg githubPRPluginConfig, sha string) string {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s/check-runs?per_page=50", cfg.Owner, cfg.Repo, sha)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "token "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return ""
	}
	defer resp.Body.Close()

	var result struct {
		CheckRuns []githubCheckRun `json:"check_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	for _, cr := range result.CheckRuns {
		if cr.Conclusion == "failure" || cr.Conclusion == "timed_out" {
			return "pr:ci-failing"
		}
	}
	return ""
}

// MapToBacklogItem converts a GitHub PR ExternalItem to a BacklogItemData.
func (g *GitHubPRsPlugin) MapToBacklogItem(item ExternalItem, sourceID string) BacklogItemData {
	title := item.Title
	if len(title) > 200 {
		title = title[:200]
	}
	desc := item.Description
	if len(desc) > 2000 {
		desc = desc[:2000]
	}
	return BacklogItemData{
		Title:       title,
		Description: desc,
		Priority:    item.Priority,
		Status:      string(BacklogStatusIdea),
		ExternalID:  item.ExternalID,
		SourceID:    sourceID,
	}
}
