package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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
// Email/token are deliberately not config fields (AC6): they are resolved
// from the keychain, then JIRA_EMAIL/JIRA_API_TOKEN, and never read from or
// written to Raw. Cloud only.
type jiraPluginConfig struct {
	BaseURL    string `json:"base_url"`
	ProjectKey string `json:"project_key"`
	Email      string `json:"-"`
	Token      string `json:"-"`
}

// resolveJiraCredentials fills cfg.Email/Token from the keychain (keyed by
// base URL), falling back to the JIRA_* environment variables.
func resolveJiraCredentials(cfg *jiraPluginConfig) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = os.Getenv("JIRA_BASE_URL")
	}
	if email, token, ok := jira.GetKeychainTokenForHost(cfg.BaseURL); ok {
		cfg.Email, cfg.Token = email, token
		return
	}
	cfg.Email, cfg.Token = os.Getenv("JIRA_EMAIL"), os.Getenv("JIRA_API_TOKEN")
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
	resolveJiraCredentials(&cfg)
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

var jiraIssueKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*-\d+$`)

// ParseJiraIssueRef extracts (baseURL, key) from a JIRA browse URL
// (https://org.atlassian.net/browse/ENG-123). A bare key ("ENG-123") returns
// an empty baseURL, which the caller must supply separately.
func ParseJiraIssueRef(ref string) (baseURL, key string, err error) {
	ref = strings.TrimSpace(ref)
	if jiraIssueKeyRe.MatchString(ref) {
		return "", strings.ToUpper(ref), nil
	}
	u, perr := url.Parse(ref)
	if perr != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", "", fmt.Errorf("not a JIRA issue URL or key: %q", ref)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, p := range parts {
		if p == "browse" && i+1 < len(parts) && jiraIssueKeyRe.MatchString(parts[i+1]) {
			return u.Scheme + "://" + u.Host, strings.ToUpper(parts[i+1]), nil
		}
	}
	return "", "", fmt.Errorf("no issue key found in JIRA URL %q", ref)
}

// FetchIssue retrieves one JIRA issue by key for one-off import. config needs
// base_url only (project_key is not required); credentials come from the
// keychain (or legacy config fields) as in Fetch.
func (j *JiraPlugin) FetchIssue(ctx context.Context, config PluginConfig, key string) (ExternalItem, error) {
	var cfg jiraPluginConfig
	if config.Raw != "" {
		if err := json.Unmarshal([]byte(config.Raw), &cfg); err != nil {
			return ExternalItem{}, fmt.Errorf("jira: parse config: %w", err)
		}
	}
	resolveJiraCredentials(&cfg)
	if cfg.BaseURL == "" {
		return ExternalItem{}, fmt.Errorf("jira: base_url is required")
	}
	if cfg.Email == "" || cfg.Token == "" {
		return ExternalItem{}, fmt.Errorf("jira: credentials not configured for %s (store email and API token in the keychain)", jira.NormalizeHost(cfg.BaseURL))
	}
	cfg.BaseURL = strings.TrimSuffix(strings.TrimSpace(cfg.BaseURL), "/")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfg.BaseURL+"/rest/api/3/issue/"+url.PathEscape(key)+"?fields=summary,description,labels,priority,updated", nil)
	if err != nil {
		return ExternalItem{}, fmt.Errorf("jira: build request: %w", err)
	}
	req.SetBasicAuth(cfg.Email, cfg.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := jiraHTTPClient.Do(req)
	if err != nil {
		return ExternalItem{}, fmt.Errorf("jira: request failed: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ExternalItem{}, fmt.Errorf("jira: issue %s not found", key)
	case resp.StatusCode == http.StatusTooManyRequests:
		return ExternalItem{}, fmt.Errorf("jira: rate limited (status %d)", resp.StatusCode)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ExternalItem{}, fmt.Errorf("jira: authentication failed (%d)", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return ExternalItem{}, fmt.Errorf("jira: unexpected status %d", resp.StatusCode)
	}
	var is jiraIssue
	if err := json.NewDecoder(resp.Body).Decode(&is); err != nil {
		return ExternalItem{}, fmt.Errorf("jira: decode response: %w", err)
	}
	items, _ := convertJiraIssues([]jiraIssue{is}, cfg.BaseURL, "")
	return items[0], nil
}
