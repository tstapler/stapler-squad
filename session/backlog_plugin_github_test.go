package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// withGitHubTestServer points githubAPIBaseURL at ts for the duration of the test.
func withGitHubTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(handler)
	orig := githubAPIBaseURL
	githubAPIBaseURL = ts.URL
	t.Cleanup(func() {
		githubAPIBaseURL = orig
		ts.Close()
	})
	return ts
}

func TestGitHubIssuesPlugin_PluginID(t *testing.T) {
	require.Equal(t, "github_issues", NewGitHubIssuesPlugin().PluginID())
}

func TestGitHubIssuesPlugin_Fetch_ReturnsEmptyWhenTokenMissing(t *testing.T) {
	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets"}`}
	items, cursor, err := p.Fetch(context.Background(), cfg, "old-cursor")
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, "old-cursor", cursor)
}

func TestGitHubIssuesPlugin_Fetch_ErrorsWhenOwnerOrRepoMissing(t *testing.T) {
	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"token":"tok"}`}
	_, _, err := p.Fetch(context.Background(), cfg, "")
	require.Error(t, err)
}

func TestGitHubIssuesPlugin_Fetch_ParsesIssuesAndComputesPriority(t *testing.T) {
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "token tok", r.Header.Get("Authorization"))
		require.Equal(t, "2024-01-01", r.URL.Query().Get("since"))
		w.Write([]byte(`[
			{"number":1,"title":"Bug A","body":"desc A","updated_at":"2024-01-02","html_url":"https://x/1","labels":[{"name":"p1"}]},
			{"number":2,"title":"Bug B","body":"desc B","updated_at":"2024-01-03","html_url":"https://x/2","labels":[{"name":"other"}]}
		]`))
	})

	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok","label_priority_map":{"p1":1}}`}
	items, newCursor, err := p.Fetch(context.Background(), cfg, "2024-01-01")
	require.NoError(t, err)
	require.Len(t, items, 2)

	require.Equal(t, "1", items[0].ExternalID)
	require.Equal(t, 1, items[0].Priority) // mapped via label_priority_map
	require.Equal(t, "other", items[1].Labels[0])
	require.Equal(t, 3, items[1].Priority) // default priority, no map entry

	// Cursor advances to the latest updated_at seen.
	require.Equal(t, "2024-01-03", newCursor)
}

func TestGitHubIssuesPlugin_Fetch_RateLimited(t *testing.T) {
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok"}`}
	_, _, err := p.Fetch(context.Background(), cfg, "")
	require.Error(t, err)
}

func TestGitHubIssuesPlugin_MapToBacklogItem_TruncatesLongFields(t *testing.T) {
	p := NewGitHubIssuesPlugin()
	longTitle := make([]byte, 300)
	longDesc := make([]byte, 3000)
	for i := range longTitle {
		longTitle[i] = 'a'
	}
	for i := range longDesc {
		longDesc[i] = 'b'
	}

	item := ExternalItem{
		ExternalID:  "42",
		Title:       string(longTitle),
		Description: string(longDesc),
		Priority:    2,
	}
	data := p.MapToBacklogItem(item, "src-1")
	require.Len(t, data.Title, 200)
	require.Len(t, data.Description, 2000)
	require.Equal(t, "42", data.ExternalID)
	require.Equal(t, "src-1", data.SourceID)
	require.Equal(t, string(BacklogStatusIdea), data.Status)
}

func TestGitHubPRsPlugin_PluginID(t *testing.T) {
	require.Equal(t, "github_prs", NewGitHubPRsPlugin().PluginID())
}

func TestGitHubPRsPlugin_Fetch_ReturnsEmptyWhenTokenMissing(t *testing.T) {
	p := NewGitHubPRsPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets"}`}
	items, _, err := p.Fetch(context.Background(), cfg, "")
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestGitHubPRsPlugin_Fetch_ParsesPRsWithReviewRequestedAndCILabels(t *testing.T) {
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/acme/widgets/pulls":
			w.Write([]byte(`[{
				"number": 7,
				"title": "Add feature",
				"body": "does a thing",
				"html_url": "https://x/7",
				"head": {"sha": "deadbeef"},
				"requested_reviewers": [{"login": "reviewer1"}]
			}]`))
		case r.URL.Path == "/repos/acme/widgets/commits/deadbeef/check-runs":
			w.Write([]byte(`{"check_runs":[{"conclusion":"failure"}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})

	p := NewGitHubPRsPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok"}`}
	items, _, err := p.Fetch(context.Background(), cfg, "")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "7", items[0].ExternalID)
	require.Contains(t, items[0].Labels, "pr:review-requested")
	require.Contains(t, items[0].Labels, "pr:ci-failing")
}

func TestGitHubPRsPlugin_MapToBacklogItem_TruncatesLongFields(t *testing.T) {
	p := NewGitHubPRsPlugin()
	longTitle := make([]byte, 300)
	for i := range longTitle {
		longTitle[i] = 'a'
	}
	item := ExternalItem{ExternalID: "9", Title: string(longTitle), Priority: 3}
	data := p.MapToBacklogItem(item, "src-2")
	require.Len(t, data.Title, 200)
	require.Equal(t, "9", data.ExternalID)
}
