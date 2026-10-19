package session

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/linear"
)

const linearIssuesPerPage = 50

// maxLinearFetchPages bounds one sync tick's pagination (var so tests can
// shrink it). Linear can only order by updatedAt descending, so a truncated
// fetch would lose the oldest changes if the cursor still advanced; Fetch
// errors out instead.
var maxLinearFetchPages = 200

// linearGraphQLURL is overridden in tests to point at an httptest server;
// tests that mutate it must not run with t.Parallel().
var linearGraphQLURL = "https://api.linear.app/graphql"

// Redirects are refused: the API never needs them, and a followed same-site
// redirect would carry the Authorization header along.
var linearHTTPClient = &http.Client{
	Timeout:       30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// linearPluginConfig is the decoded PluginConfig.Raw for the Linear plugin.
// The API key is deliberately not a config field (AC6): it is resolved from
// the keychain, then LINEAR_API_KEY, and never read from or written to Raw.
type linearPluginConfig struct {
	TeamKey string `json:"team_key"`
	TeamID  string `json:"team_id"`
	Token   string `json:"-"`
}

type linearIssue struct {
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Priority    int    `json:"priority"`
	UpdatedAt   string `json:"updatedAt"`
	State       struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"state"`
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
}

const linearIssuesQuery = `query Issues($first: Int!, $after: String, $filter: IssueFilter) {
  issues(first: $first, after: $after, filter: $filter, orderBy: updatedAt) {
    nodes { identifier title description url priority updatedAt state { name type } labels { nodes { name } } }
    pageInfo { hasNextPage endCursor }
  }
}`

// LinearPlugin fetches backlog items from a Linear workspace via GraphQL.
type LinearPlugin struct{}

// NewLinearPlugin returns a new LinearPlugin.
func NewLinearPlugin() *LinearPlugin { return &LinearPlugin{} }

// PluginID returns the unique identifier for this plugin.
func (l *LinearPlugin) PluginID() string { return "linear" }

// decodeLinearFetchConfig parses config; disabled is true when no API key is
// available (source disabled, not an error).
func decodeLinearFetchConfig(config PluginConfig) (cfg linearPluginConfig, disabled bool, err error) {
	if config.Raw != "" {
		if err := json.Unmarshal([]byte(config.Raw), &cfg); err != nil {
			return linearPluginConfig{}, false, fmt.Errorf("linear: parse config: %w", err)
		}
	}
	cfg.Token = linear.GetKeychainToken()
	if cfg.Token == "" {
		cfg.Token = os.Getenv("LINEAR_API_KEY")
	}
	if cfg.Token == "" {
		return linearPluginConfig{}, true, nil
	}
	return cfg, false, nil
}

// Fetch returns Linear issues updated after cursor (an RFC3339 updatedAt) and
// the advanced cursor. An empty cursor fetches everything.
func (l *LinearPlugin) Fetch(ctx context.Context, config PluginConfig, cursor string) ([]ExternalItem, string, error) {
	cfg, disabled, err := decodeLinearFetchConfig(config)
	if err != nil {
		return nil, cursor, err
	}
	if disabled {
		return nil, cursor, nil
	}

	filter := map[string]any{}
	if cursor != "" {
		filter["updatedAt"] = map[string]any{"gt": cursor}
	}
	switch {
	case cfg.TeamID != "":
		filter["team"] = map[string]any{"id": map[string]any{"eq": cfg.TeamID}}
	case cfg.TeamKey != "":
		filter["team"] = map[string]any{"key": map[string]any{"eq": cfg.TeamKey}}
	}

	var all []linearIssue
	var after any
	for page := 1; ; page++ {
		if page > maxLinearFetchPages {
			return nil, cursor, fmt.Errorf("linear: more than %d pages of changes in one sync; refusing to advance cursor", maxLinearFetchPages)
		}
		var out struct {
			Issues struct {
				Nodes    []linearIssue `json:"nodes"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"issues"`
		}
		vars := map[string]any{"first": linearIssuesPerPage, "after": after, "filter": filter}
		if err := doLinearGraphQL(ctx, cfg.Token, linearIssuesQuery, vars, &out); err != nil {
			return nil, cursor, err
		}
		all = append(all, out.Issues.Nodes...)
		if !out.Issues.PageInfo.HasNextPage {
			break
		}
		after = out.Issues.PageInfo.EndCursor
	}

	items, newCursor := convertLinearIssues(all, cursor)
	return items, newCursor, nil
}

// doLinearGraphQL POSTs one GraphQL operation. Linear takes the raw API key
// in Authorization with NO "Bearer"/"token" prefix (OAuth tokens would need
// "Bearer"); do not unify this with the GitHub helper. No error path may
// include the key.
func doLinearGraphQL(ctx context.Context, token, query string, variables map[string]any, out any) error {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return fmt.Errorf("linear: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, linearGraphQLURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("linear: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)

	resp, err := linearHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("linear: request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTrackerResponseBytes))
	if err != nil {
		return fmt.Errorf("linear: read response: %w", err)
	}

	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				Type string `json:"type"`
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	decodeErr := json.Unmarshal(body, &env)

	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("linear: rate limited (status %d)", resp.StatusCode)
	}
	if len(env.Errors) > 0 {
		e := env.Errors[0]
		kind := strings.ToLower(e.Extensions.Type + " " + e.Extensions.Code)
		switch {
		case strings.Contains(kind, "ratelimit"):
			return fmt.Errorf("linear: rate limited: %s", e.Message)
		case strings.Contains(kind, "authentication"), strings.Contains(kind, "forbidden"), resp.StatusCode == http.StatusUnauthorized:
			// Literal "401" so the frontend's isAuthFailure() banner matches.
			return fmt.Errorf("linear: authentication failed (401): %s", e.Message)
		}
		return fmt.Errorf("linear: graphql error: %s", e.Message)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("linear: authentication failed (401): status %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("linear: unexpected status %d: %.512s", resp.StatusCode, strings.ToValidUTF8(string(body), ""))
	}
	if decodeErr != nil {
		return fmt.Errorf("linear: decode response: %w", decodeErr)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("linear: decode data: %w", err)
	}
	return nil
}

// convertLinearIssues maps raw issues to ExternalItems and returns the max
// updatedAt seen (RFC3339 UTC strings sort lexicographically). ExternalItem.State
// stays "" — backward status sync is out of scope for Linear.
func convertLinearIssues(issues []linearIssue, cursor string) ([]ExternalItem, string) {
	if len(issues) == 0 {
		return nil, cursor
	}
	items := make([]ExternalItem, 0, len(issues))
	newCursor := cursor
	for _, is := range issues {
		labels := make([]string, 0, len(is.Labels.Nodes))
		for _, n := range is.Labels.Nodes {
			labels = append(labels, n.Name)
		}
		updated, _ := time.Parse(time.RFC3339, is.UpdatedAt)
		items = append(items, ExternalItem{
			ExternalID:     is.Identifier,
			Title:          is.Title,
			Description:    is.Description,
			Labels:         labels,
			Priority:       linearPriorityToBacklog(is.Priority),
			URL:            is.URL,
			IssueUpdatedAt: updated,
		})
		if is.UpdatedAt > newCursor {
			newCursor = is.UpdatedAt
		}
	}
	return items, newCursor
}

// linearPriorityToBacklog maps Linear's 0-4 (0 = none, 1 = urgent) to the
// backlog's 1-5 scale (5 = highest).
func linearPriorityToBacklog(p int) int {
	switch p {
	case 1:
		return 5
	case 2:
		return 4
	case 4:
		return 2
	default: // 0 (no priority) and 3 (medium)
		return DefaultBacklogPriority
	}
}

// MapToBacklogItem converts a Linear ExternalItem to a BacklogItemData.
func (l *LinearPlugin) MapToBacklogItem(item ExternalItem, sourceID string) BacklogItemData {
	return mapExternalToBacklogItem(item, sourceID)
}

const linearIssueByIDQuery = `query Issue($id: String!) {
  issue(id: $id) {
    identifier title description url priority updatedAt state { name type } labels { nodes { name } }
  }
}`

var linearIdentifierRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-\d+$`)

// ParseLinearIssueRef extracts the issue identifier ("ENG-123") from either a
// Linear issue URL (https://linear.app/<org>/issue/ENG-123[/slug]) or a bare
// identifier.
func ParseLinearIssueRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if linearIdentifierRe.MatchString(ref) {
		return strings.ToUpper(ref), nil
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host != "linear.app" {
		return "", fmt.Errorf("not a Linear issue URL or identifier: %q", ref)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, p := range parts {
		if p == "issue" && i+1 < len(parts) && linearIdentifierRe.MatchString(parts[i+1]) {
			return strings.ToUpper(parts[i+1]), nil
		}
	}
	return "", fmt.Errorf("no issue identifier found in Linear URL %q", ref)
}

// FetchIssue retrieves one Linear issue by identifier for one-off import.
func (l *LinearPlugin) FetchIssue(ctx context.Context, config PluginConfig, identifier string) (ExternalItem, error) {
	cfg, disabled, err := decodeLinearFetchConfig(config)
	if err != nil {
		return ExternalItem{}, err
	}
	if disabled {
		return ExternalItem{}, fmt.Errorf("linear: API key not configured (set LINEAR_API_KEY or store it in the keychain)")
	}
	var out struct {
		Issue *linearIssue `json:"issue"`
	}
	if err := doLinearGraphQL(ctx, cfg.Token, linearIssueByIDQuery, map[string]any{"id": identifier}, &out); err != nil {
		return ExternalItem{}, err
	}
	if out.Issue == nil || out.Issue.Identifier == "" {
		return ExternalItem{}, fmt.Errorf("linear: issue %s not found", identifier)
	}
	items, _ := convertLinearIssues([]linearIssue{*out.Issue}, "")
	return items[0], nil
}
