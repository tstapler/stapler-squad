package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/jira"
)

const jiraIssuesPerPage = 50

// maxJiraFetchPages bounds one sync tick's pagination (var so tests can
// shrink it). Results are ordered by updated ASC, so a truncated fetch just
// resumes from the advanced cursor on the next tick.
var maxJiraFetchPages = 20

var jiraHTTPClient = &http.Client{Timeout: 30 * time.Second}

// jiraCursorLayout is JQL's minute-resolution date literal. The cursor is the
// max `updated` seen, rendered in the issue's own UTC offset (the same
// timezone JQL interprets the literal in for that user).
const jiraCursorLayout = "2006-01-02 15:04"

// jiraTimeLayouts are the timestamp shapes JIRA returns in Fields.Updated.
var jiraTimeLayouts = []string{"2006-01-02T15:04:05.000-0700", time.RFC3339Nano, time.RFC3339}

var jiraProjectKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// jiraPluginConfig is the decoded PluginConfig.Raw for the JIRA plugin.
// Email/token are deliberately absent: they live in the keychain (AC6), with
// a legacy per-source fallback mirroring the GitHub plugin. Cloud only.
type jiraPluginConfig struct {
	BaseURL    string `json:"base_url"`
	ProjectKey string `json:"project_key"`
	Email      string `json:"email"`
	Token      string `json:"token"`
}

type jiraIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary     string          `json:"summary"`
		Description json.RawMessage `json:"description"`
		Labels      []string        `json:"labels"`
		Updated     string          `json:"updated"`
		Priority    *struct {
			Name string `json:"name"`
		} `json:"priority"`
	} `json:"fields"`
}

// JiraPlugin fetches backlog items from a JIRA Cloud project via REST v3.
type JiraPlugin struct{}

// NewJiraPlugin returns a new JiraPlugin.
func NewJiraPlugin() *JiraPlugin { return &JiraPlugin{} }

// PluginID returns the unique identifier for this plugin.
func (j *JiraPlugin) PluginID() string { return "jira" }

// decodeJiraFetchConfig parses config; disabled is true when credentials are
// absent (source disabled, not an error).
func decodeJiraFetchConfig(config PluginConfig) (cfg jiraPluginConfig, disabled bool, err error) {
	if config.Raw != "" {
		if err := json.Unmarshal([]byte(config.Raw), &cfg); err != nil {
			return jiraPluginConfig{}, false, fmt.Errorf("jira: parse config: %w", err)
		}
	}
	if email, token, ok := jira.GetKeychainTokenForHost(cfg.BaseURL); ok {
		cfg.Email, cfg.Token = email, token
	}
	if cfg.Token == "" || cfg.Email == "" {
		return jiraPluginConfig{}, true, nil
	}
	if cfg.BaseURL == "" || cfg.ProjectKey == "" {
		return jiraPluginConfig{}, false, fmt.Errorf("jira: base_url and project_key are required in config")
	}
	if !jiraProjectKeyRe.MatchString(cfg.ProjectKey) {
		return jiraPluginConfig{}, false, fmt.Errorf("jira: invalid project_key %q", cfg.ProjectKey)
	}
	cfg.BaseURL = strings.TrimSuffix(strings.TrimSpace(cfg.BaseURL), "/")
	return cfg, false, nil
}

func jiraBuildJQL(projectKey, cursor string) string {
	jql := "project = " + projectKey
	if cursor != "" {
		jql += fmt.Sprintf(" AND updated >= %q", cursor)
	}
	return jql + " ORDER BY updated ASC"
}

// Fetch returns JIRA issues updated at or after cursor ("2006-01-02 15:04")
// and the advanced cursor, following nextPageToken across all pages so a busy
// tick isn't silently truncated to its first page.
func (j *JiraPlugin) Fetch(ctx context.Context, config PluginConfig, cursor string) ([]ExternalItem, string, error) {
	cfg, disabled, err := decodeJiraFetchConfig(config)
	if err != nil {
		return nil, cursor, err
	}
	if disabled {
		return nil, cursor, nil
	}

	jql := jiraBuildJQL(cfg.ProjectKey, cursor)
	var all []jiraIssue
	pageToken := ""
	for page := 1; page <= maxJiraFetchPages; page++ {
		issues, next, err := fetchJiraPage(ctx, cfg, jql, pageToken)
		if err != nil {
			return nil, cursor, err
		}
		all = append(all, issues...)
		if next == "" {
			break
		}
		pageToken = next
	}

	items, newCursor := convertJiraIssues(all, cfg.BaseURL, cursor)
	return items, newCursor, nil
}

func fetchJiraPage(ctx context.Context, cfg jiraPluginConfig, jql, pageToken string) ([]jiraIssue, string, error) {
	q := url.Values{}
	q.Set("jql", jql)
	q.Set("maxResults", strconv.Itoa(jiraIssuesPerPage))
	q.Set("fields", "summary,description,labels,priority,updated")
	if pageToken != "" {
		q.Set("nextPageToken", pageToken)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.BaseURL+"/rest/api/3/search/jql?"+q.Encode(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("jira: build request: %w", err)
	}
	req.SetBasicAuth(cfg.Email, cfg.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := jiraHTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("jira: request failed: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, "", fmt.Errorf("jira: rate limited (status %d, retry-after %q)", resp.StatusCode, resp.Header.Get("Retry-After"))
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, "", fmt.Errorf("jira: authentication failed (%d)", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, "", fmt.Errorf("jira: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var out struct {
		Issues        []jiraIssue `json:"issues"`
		NextPageToken string      `json:"nextPageToken"`
		IsLast        bool        `json:"isLast"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, "", fmt.Errorf("jira: decode response: %w", err)
	}
	if out.IsLast {
		out.NextPageToken = ""
	}
	return out.Issues, out.NextPageToken, nil
}

func parseJiraTime(s string) (time.Time, bool) {
	for _, layout := range jiraTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// convertJiraIssues maps raw issues to ExternalItems and returns the max
// `updated` seen as a JQL-ready cursor. ExternalItem.State stays "" —
// backward status sync is out of scope for JIRA.
func convertJiraIssues(issues []jiraIssue, baseURL, cursor string) ([]ExternalItem, string) {
	if len(issues) == 0 {
		return nil, cursor
	}
	items := make([]ExternalItem, 0, len(issues))
	newCursor := cursor
	for _, is := range issues {
		updated, ok := parseJiraTime(is.Fields.Updated)
		prio := ""
		if is.Fields.Priority != nil {
			prio = is.Fields.Priority.Name
		}
		items = append(items, ExternalItem{
			ExternalID:     is.Key,
			Title:          is.Fields.Summary,
			Description:    jira.ADFToPlainText(is.Fields.Description),
			Labels:         is.Fields.Labels,
			Priority:       jiraPriorityToBacklog(prio),
			URL:            baseURL + "/browse/" + is.Key,
			IssueUpdatedAt: updated,
		})
		if ok {
			if c := updated.Format(jiraCursorLayout); c > newCursor {
				newCursor = c
			}
		}
	}
	return items, newCursor
}

// jiraPriorityToBacklog maps JIRA's default priority names to the backlog's
// 1-5 scale; unknown/absent priorities get the default.
func jiraPriorityToBacklog(name string) int {
	switch strings.ToLower(name) {
	case "highest", "blocker":
		return 5
	case "high", "critical":
		return 4
	case "low", "minor":
		return 2
	case "lowest", "trivial":
		return 1
	default:
		return DefaultBacklogPriority
	}
}

// MapToBacklogItem converts a JIRA ExternalItem to a BacklogItemData.
func (j *JiraPlugin) MapToBacklogItem(item ExternalItem, sourceID string) BacklogItemData {
	return mapExternalToBacklogItem(item, sourceID)
}
