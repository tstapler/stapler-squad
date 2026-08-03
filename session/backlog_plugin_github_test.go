package session

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

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

// TestGitHubIssuesPlugin_Fetch_IssueUpdatedAtMatchesCursorValue verifies that
// ExternalItem.IssueUpdatedAt (Epic 0.2, Story 0.2.2) is parsed from the same
// updated_at value used to compute newCursor for that same issue — i.e. it's
// the already-observed timestamp, not independently re-derived.
func TestGitHubIssuesPlugin_Fetch_IssueUpdatedAtMatchesCursorValue(t *testing.T) {
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"number":1,"title":"Bug A","body":"desc A","updated_at":"2024-01-02T10:00:00Z","html_url":"https://x/1"},
			{"number":2,"title":"Bug B","body":"desc B","updated_at":"2024-01-03T15:30:00Z","html_url":"https://x/2"}
		]`))
	})

	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok"}`}
	items, newCursor, err := p.Fetch(context.Background(), cfg, "")
	require.NoError(t, err)
	require.Len(t, items, 2)

	require.Equal(t, "2024-01-03T15:30:00Z", newCursor)
	require.True(t, items[0].IssueUpdatedAt.Equal(time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)))
	require.True(t, items[1].IssueUpdatedAt.Equal(time.Date(2024, 1, 3, 15, 30, 0, 0, time.UTC)))
	require.Equal(t, newCursor, items[1].IssueUpdatedAt.Format(time.RFC3339))
}

// TestGitHubIssuesPlugin_Fetch_IncludesClosedIssues verifies Fetch's query
// switched from state=open to state=all (Epic 0.2, Story 0.2.1 / AC0): a
// closed issue in the mock response — previously invisible to Fetch — now
// appears in the returned []ExternalItem, with State decoded correctly for
// both the open and closed issue.
func TestGitHubIssuesPlugin_Fetch_IncludesClosedIssues(t *testing.T) {
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "all", r.URL.Query().Get("state"))
		w.Write([]byte(`[
			{"number":1,"title":"Open issue","body":"still open","state":"open","updated_at":"2024-01-02T00:00:00Z","html_url":"https://x/1"},
			{"number":2,"title":"Closed issue","body":"already closed","state":"closed","updated_at":"2024-01-03T00:00:00Z","html_url":"https://x/2"}
		]`))
	})

	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok"}`}
	items, _, err := p.Fetch(context.Background(), cfg, "")
	require.NoError(t, err)
	require.Len(t, items, 2)

	require.Equal(t, "1", items[0].ExternalID)
	require.Equal(t, "open", items[0].State)
	require.Equal(t, "2", items[1].ExternalID)
	require.Equal(t, "closed", items[1].State)
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
		Labels:      []string{"bug", "p1"},
		URL:         "https://github.com/acme/widgets/issues/42",
	}
	data := p.MapToBacklogItem(item, "src-1")
	require.Len(t, data.Title, 200)
	require.Len(t, data.Description, 2000)
	require.Equal(t, "42", data.ExternalID)
	require.Equal(t, "src-1", data.SourceID)
	require.Equal(t, string(BacklogStatusIdea), data.Status)
	// Epic 0.1 (Story 0.1.3): Labels and ExternalURL must pass through unchanged.
	require.Equal(t, []string{"bug", "p1"}, data.Labels)
	require.Equal(t, "https://github.com/acme/widgets/issues/42", data.ExternalURL)
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
		switch r.URL.Path {
		case "/repos/acme/widgets/pulls":
			w.Write([]byte(`[{
				"number": 7,
				"title": "Add feature",
				"body": "does a thing",
				"html_url": "https://x/7",
				"head": {"sha": "deadbeef"},
				"requested_reviewers": [{"login": "reviewer1"}]
			}]`))
		case "/repos/acme/widgets/commits/deadbeef/check-runs":
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

// TestGitHubPRsPlugin_Fetch_ConcurrentCIFetchPreservesPerPRLabels verifies that
// Fetch's bounded-concurrency CI-label lookup (githubCILabelConcurrency workers)
// still returns items in the original PR order with each PR's own labels correctly
// matched, not swapped or dropped, despite fetching check-runs concurrently. Run
// with -race to confirm the concurrent writes into labelsByIndex are race-free.
func TestGitHubPRsPlugin_Fetch_ConcurrentCIFetchPreservesPerPRLabels(t *testing.T) {
	const prCount = 12
	var inFlight, maxInFlight int32
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widgets/pulls":
			var sb []byte
			sb = append(sb, '[')
			for i := 0; i < prCount; i++ {
				if i > 0 {
					sb = append(sb, ',')
				}
				sb = append(sb, []byte(`{"number":`+strconv.Itoa(i)+`,"title":"pr `+strconv.Itoa(i)+`","html_url":"https://x/`+strconv.Itoa(i)+`","head":{"sha":"sha-`+strconv.Itoa(i)+`"}}`)...)
			}
			sb = append(sb, ']')
			w.Write(sb)
		default:
			// /repos/acme/widgets/commits/sha-N/check-runs — even N fails CI, odd N passes.
			// Track concurrent in-flight requests to prove the fetch is actually bounded
			// AND parallel (not accidentally serialized), not just correct in isolation.
			n := atomic.AddInt32(&inFlight, 1)
			defer atomic.AddInt32(&inFlight, -1)
			for {
				old := atomic.LoadInt32(&maxInFlight)
				if n <= old || atomic.CompareAndSwapInt32(&maxInFlight, old, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond) // widen the overlap window so concurrent requests actually coincide

			var num int
			fmt.Sscanf(r.URL.Path, "/repos/acme/widgets/commits/sha-%d/check-runs", &num)
			if num%2 == 0 {
				w.Write([]byte(`{"check_runs":[{"conclusion":"failure"}]}`))
			} else {
				w.Write([]byte(`{"check_runs":[{"conclusion":"success"}]}`))
			}
		}
	})

	p := NewGitHubPRsPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok"}`}
	items, _, err := p.Fetch(context.Background(), cfg, "")
	require.NoError(t, err)
	require.Len(t, items, prCount)

	for i, item := range items {
		require.Equal(t, strconv.Itoa(i), item.ExternalID, "PR order must be preserved despite concurrent CI fetch")
		if i%2 == 0 {
			require.Contains(t, item.Labels, "pr:ci-failing", "PR %d should be labeled ci-failing", i)
		} else {
			require.NotContains(t, item.Labels, "pr:ci-failing", "PR %d should not be labeled ci-failing", i)
		}
	}

	observed := int(atomic.LoadInt32(&maxInFlight))
	require.Greater(t, observed, 1, "fetches should actually overlap — a regression to serial fetching would not be caught otherwise")
	require.LessOrEqual(t, observed, githubCILabelConcurrency, "concurrency must stay bounded by githubCILabelConcurrency")
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
