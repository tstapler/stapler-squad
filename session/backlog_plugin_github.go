package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/github"
)

const githubIssuesPerPage = 50

// githubAPIBaseURL is the default base URL used when a plugin config doesn't
// specify a Host. Overridden in tests (session/backlog_plugin_github_test.go)
// to point at an httptest server; shared by both the Issues and PRs plugins.
// Tests that mutate it must not run with t.Parallel() — it's package-level
// mutable state with no synchronization.
var githubAPIBaseURL = "https://api.github.com"

// githubAPIURL joins the REST API base URL for host with a path, tolerating a
// trailing slash on the base URL so "//repos/..." can't slip in. An empty
// host resolves to githubAPIBaseURL (github.com, or the test override).
func githubAPIURL(host, pathAndQuery string) string {
	base := githubAPIBaseURL
	if host != "" && !github.IsGitHubCom(host) {
		base = github.RestBaseURLForHost(host)
	}
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(pathAndQuery, "/")
}

// githubPluginConfig holds the decoded config for the GitHub Issues plugin.
type githubPluginConfig struct {
	Host             string         `json:"host"`
	Owner            string         `json:"owner"`
	Repo             string         `json:"repo"`
	Token            string         `json:"token"`
	LabelPriorityMap map[string]int `json:"label_priority_map"`
}

// githubIssue is the subset of fields from the GitHub Issues API response.
type githubIssue struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	UpdatedAt string `json:"updated_at"`
	HTMLURL   string `json:"html_url"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// GitHubIssuesPlugin fetches backlog items from a GitHub repository's issue tracker.
type GitHubIssuesPlugin struct{}

// NewGitHubIssuesPlugin returns a new GitHubIssuesPlugin.
func NewGitHubIssuesPlugin() *GitHubIssuesPlugin {
	return &GitHubIssuesPlugin{}
}

// PluginID returns the unique identifier for this plugin.
func (g *GitHubIssuesPlugin) PluginID() string {
	return "github_issues"
}

// Fetch retrieves new and updated GitHub issues since the cursor. The cursor is
// an ISO 8601 timestamp passed as the `since` query parameter. Returns the
// updated cursor (the most recent updated_at seen) and the fetched items.
// If the token field is empty, Fetch returns an empty list and the original cursor.
func (g *GitHubIssuesPlugin) Fetch(ctx context.Context, config PluginConfig, cursor string) ([]ExternalItem, string, error) {
	var cfg githubPluginConfig
	if config.Raw != "" {
		if err := json.Unmarshal([]byte(config.Raw), &cfg); err != nil {
			return nil, cursor, fmt.Errorf("github_issues: parse config: %w", err)
		}
	}

	// Token is required; disabled when absent.
	if cfg.Token == "" {
		return nil, cursor, nil
	}
	if cfg.Owner == "" || cfg.Repo == "" {
		return nil, cursor, fmt.Errorf("github_issues: owner and repo are required in config")
	}

	url := githubAPIURL(cfg.Host, fmt.Sprintf("repos/%s/%s/issues?state=all&per_page=%d", cfg.Owner, cfg.Repo, githubIssuesPerPage))
	if cursor != "" {
		url += "&since=" + cursor
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, cursor, fmt.Errorf("github_issues: build request: %w", err)
	}
	req.Header.Set("Authorization", "token "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, cursor, fmt.Errorf("github_issues: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle rate limiting.
	if resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0") {
		return nil, cursor, fmt.Errorf("github_issues: rate limited (status %d)", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, cursor, fmt.Errorf("github_issues: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var issues []githubIssue
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		return nil, cursor, fmt.Errorf("github_issues: decode response: %w", err)
	}

	if len(issues) == 0 {
		return nil, cursor, nil
	}

	items := make([]ExternalItem, 0, len(issues))
	newCursor := cursor

	for _, issue := range issues {
		// Compute priority from label map.
		priority := 3
		for _, label := range issue.Labels {
			if p, ok := cfg.LabelPriorityMap[label.Name]; ok {
				priority = p
				break
			}
		}

		labelNames := make([]string, len(issue.Labels))
		for i, l := range issue.Labels {
			labelNames[i] = l.Name
		}

		// Best-effort parse of updated_at for IssueUpdatedAt; if it fails to
		// parse (unexpected format), IssueUpdatedAt is left at its zero value
		// rather than failing the whole Fetch — the string form is still used
		// unmodified for the newCursor comparison below.
		issueUpdatedAt, _ := time.Parse(time.RFC3339, issue.UpdatedAt)

		items = append(items, ExternalItem{
			ExternalID:     strconv.Itoa(issue.Number),
			Title:          issue.Title,
			Description:    issue.Body,
			Labels:         labelNames,
			Priority:       priority,
			URL:            issue.HTMLURL,
			State:          issue.State,
			IssueUpdatedAt: issueUpdatedAt,
		})

		// Track latest updated_at as the new cursor.
		if issue.UpdatedAt > newCursor {
			newCursor = issue.UpdatedAt
		}
	}

	return items, newCursor, nil
}

// MapToBacklogItem converts a GitHub ExternalItem to a BacklogItemData.
func (g *GitHubIssuesPlugin) MapToBacklogItem(item ExternalItem, sourceID string) BacklogItemData {
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
		ExternalURL: item.URL,
		Labels:      item.Labels,
		SourceID:    sourceID,
	}
}
