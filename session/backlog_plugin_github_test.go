package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/github"
)

// These tests (both GitHubIssuesPlugin and GitHubPRsPlugin) rely on
// TestMain in integration_test.go calling keyring.MockInit() to keep
// github.GetKeychainTokenForHost from reading a real OS keychain entry. If
// integration_test.go is ever moved or its TestMain removed, these tests
// will silently start hitting the real GitHub API with a real token on any
// machine that has one stored.

// withGitHubTestServer points githubAPIBaseURL at ts for the duration of the test.
//
// Also resets github.DefaultRateLimiter: it's a package-level global shared
// by every test in this binary, and since rateLimitTransport.RoundTrip fails
// fast when it's already limited (github/http_client.go), a test that
// deliberately triggers a rate-limit response (e.g.
// TestGitHubIssuesPlugin_CloseIssue_RateLimitedReturnsError) otherwise
// poisons every GitHub-calling test that runs after it for up to 60s.
func withGitHubTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(handler)
	orig := githubAPIBaseURL
	githubAPIBaseURL = ts.URL
	origLimiter := github.DefaultRateLimiter
	github.DefaultRateLimiter = &github.RateLimiter{}
	t.Cleanup(func() {
		githubAPIBaseURL = orig
		github.DefaultRateLimiter = origLimiter
		ts.Close()
		github.DefaultRateLimiter.Reset()
	})
	return ts
}

func TestGitHubIssuesPlugin_PluginID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "github_issues", NewGitHubIssuesPlugin().PluginID())
}

// Also serves as the regression guard for the keychain-token leak fixed in
// 8a84747ca: relies on TestMain's keyring.MockInit() to keep
// GetKeychainTokenForHost from returning a real machine-local token instead
// of "". If MockInit is ever dropped from TestMain, this test starts failing
// on any machine with a real GitHub account connected.
func TestGitHubIssuesPlugin_Fetch_ReturnsEmptyWhenTokenMissing(t *testing.T) {
	t.Parallel()
	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets"}`}
	items, cursor, err := p.Fetch(context.Background(), cfg, "old-cursor")
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, "old-cursor", cursor)
}

func TestGitHubIssuesPlugin_Fetch_ErrorsWhenOwnerOrRepoMissing(t *testing.T) {
	t.Parallel()
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
	github.ResetRateLimiterForTest(t)
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok"}`}
	_, _, err := p.Fetch(context.Background(), cfg, "")
	require.Error(t, err)
}

// fakeGithubIssuesPage renders a JSON array of n synthetic issues, numbered
// starting at startNumber, for use as a canned page response in FetchAll
// pagination tests.
func fakeGithubIssuesPage(startNumber, n int) string {
	issues := make([]string, n)
	for i := 0; i < n; i++ {
		num := startNumber + i
		issues[i] = fmt.Sprintf(`{"number":%d,"title":"Issue %d","state":"closed","updated_at":"2024-01-01T00:00:00Z","html_url":"https://x/%d"}`, num, num, num)
	}
	return "[" + strings.Join(issues, ",") + "]"
}

// TestGitHubIssuesPlugin_FetchAll_AggregatesMultiplePages verifies FetchAll
// (the PaginatedFetcher implementation used by PreviewBackwardSyncImpact)
// follows page= across a full first page and a short second page, stopping
// once a page comes back under githubIssuesPerPage — proving it retrieves
// issues beyond what a single Fetch call (page 1 only) would see.
func TestGitHubIssuesPlugin_FetchAll_AggregatesMultiplePages(t *testing.T) {
	var pagesRequested []string
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pagesRequested = append(pagesRequested, page)
		switch page {
		case "1", "":
			w.Write([]byte(fakeGithubIssuesPage(1, githubIssuesPerPage)))
		case "2":
			w.Write([]byte(fakeGithubIssuesPage(githubIssuesPerPage+1, 10)))
		default:
			t.Fatalf("unexpected page requested: %q", page)
		}
	})

	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok"}`}
	items, _, possiblyIncomplete, err := p.FetchAll(context.Background(), cfg, "")
	require.NoError(t, err)
	require.Equal(t, []string{"1", "2"}, pagesRequested)
	require.Len(t, items, githubIssuesPerPage+10)
	require.False(t, possiblyIncomplete, "short second page means the full result set was retrieved")
	require.Equal(t, "1", items[0].ExternalID)
	require.Equal(t, strconv.Itoa(githubIssuesPerPage+10), items[len(items)-1].ExternalID)
}

// TestGitHubIssuesPlugin_FetchAll_SetsPossiblyIncompleteWhenCapHit verifies
// that when every page up to the pagination cap comes back full, FetchAll
// reports possiblyIncomplete=true rather than silently under-reporting the
// blast radius as if the result set were exhaustive — the CRITICAL finding
// this fixes: a single-page Fetch on a repo with >50 issues could report
// "0 items affected" when older closed issues existed beyond page 1.
func TestGitHubIssuesPlugin_FetchAll_SetsPossiblyIncompleteWhenCapHit(t *testing.T) {
	origCap := maxPreviewFetchPages
	maxPreviewFetchPages = 2
	t.Cleanup(func() { maxPreviewFetchPages = origCap })

	requestedPages := 0
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestedPages++
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		w.Write([]byte(fakeGithubIssuesPage((page-1)*githubIssuesPerPage+1, githubIssuesPerPage)))
	})

	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok"}`}
	items, _, possiblyIncomplete, err := p.FetchAll(context.Background(), cfg, "")
	require.NoError(t, err)
	require.Equal(t, 2, requestedPages, "should stop at the cap rather than fetching indefinitely")
	require.Len(t, items, 2*githubIssuesPerPage)
	require.True(t, possiblyIncomplete, "every page was full up to the cap, so more issues may exist beyond it")
}

// TestGitHubIssuesPlugin_FetchAll_ReturnsEmptyWhenTokenMissing mirrors
// Fetch's "disabled source" contract: FetchAll must not attempt any request
// when no token is configured.
func TestGitHubIssuesPlugin_FetchAll_ReturnsEmptyWhenTokenMissing(t *testing.T) {
	t.Parallel()
	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets"}`}
	items, cursor, possiblyIncomplete, err := p.FetchAll(context.Background(), cfg, "old-cursor")
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, "old-cursor", cursor)
	require.False(t, possiblyIncomplete)
}

func TestGitHubIssuesPlugin_MapToBacklogItem_TruncatesLongFields(t *testing.T) {
	t.Parallel()
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

// TestGitHubIssuesPlugin_CloseIssue_MergesLabels verifies that CloseIssue
// merges closeLabel into the issue's existing labels (never replacing them —
// GitHub's PATCH .../issues/{n} endpoint fully replaces the labels array, so
// existingLabels must be passed in and merged locally) — AC3, Story 1.1.1.
func TestGitHubIssuesPlugin_CloseIssue_MergesLabels(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]interface{}
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Write([]byte(`{"number":42,"state":"closed","updated_at":"2024-05-01T12:00:00Z"}`))
	})

	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok"}`}
	_, err := p.CloseIssue(context.Background(), cfg, "42", []string{"bug"}, "shipped")
	require.NoError(t, err)

	require.Equal(t, http.MethodPatch, gotMethod)
	require.Equal(t, "/repos/acme/widgets/issues/42", gotPath)
	require.Equal(t, "closed", gotBody["state"])
	require.ElementsMatch(t, []interface{}{"bug", "shipped"}, gotBody["labels"])
}

// TestGitHubIssuesPlugin_CloseIssue_NoLabelOmitsLabelsField verifies that when
// closeLabel == "", the PATCH body omits `labels` entirely rather than sending
// an empty array — GitHub interprets an empty array as "clear all labels",
// which is a different (destructive) semantic from "don't touch labels".
func TestGitHubIssuesPlugin_CloseIssue_NoLabelOmitsLabelsField(t *testing.T) {
	var gotBody map[string]interface{}
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Write([]byte(`{"number":42,"state":"closed","updated_at":"2024-05-01T12:00:00Z"}`))
	})

	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok"}`}
	_, err := p.CloseIssue(context.Background(), cfg, "42", []string{"bug"}, "")
	require.NoError(t, err)

	require.Equal(t, "closed", gotBody["state"])
	_, hasLabels := gotBody["labels"]
	require.False(t, hasLabels, "labels field must be omitted entirely, not sent as an empty array")
}

// TestGitHubIssuesPlugin_CloseIssue_RateLimitedReturnsError verifies a 403 +
// X-RateLimit-Remaining:0 response surfaces a descriptive error rather than
// panicking.
func TestGitHubIssuesPlugin_CloseIssue_RateLimitedReturnsError(t *testing.T) {
	github.ResetRateLimiterForTest(t)
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusForbidden)
	})

	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok"}`}
	_, err := p.CloseIssue(context.Background(), cfg, "42", nil, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "rate limited")
}

// TestGitHubIssuesPlugin_CloseIssue_ReturnsResponseUpdatedAt is the regression
// test for pre-mortem P1 #1: CloseIssue must return the PATCH response's own
// updated_at, not the test's (or caller's) wall-clock time. ADR-003 chose the
// per-resource-timestamp watermark specifically to avoid clock skew.
func TestGitHubIssuesPlugin_CloseIssue_ReturnsResponseUpdatedAt(t *testing.T) {
	const fixedTimestamp = "2019-03-14T09:26:53Z"
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"number":42,"state":"closed","updated_at":"` + fixedTimestamp + `"}`))
	})

	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok"}`}
	updatedAt, err := p.CloseIssue(context.Background(), cfg, "42", nil, "")
	require.NoError(t, err)

	expected, parseErr := time.Parse(time.RFC3339, fixedTimestamp)
	require.NoError(t, parseErr)
	require.True(t, updatedAt.Equal(expected), "expected %v, got %v (must not be wall-clock time)", expected, updatedAt)
	require.False(t, updatedAt.Equal(time.Now()), "must not equal wall-clock time")
}

// TestGitHubIssuesPlugin_PostIssueComment_SendsExpectedBody verifies the bot
// comment left on an automated close (AC3, Story 1.1.2 — "no silent automated
// action" convention).
func TestGitHubIssuesPlugin_PostIssueComment_SendsExpectedBody(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string
	withGitHubTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Write([]byte(`{"id":1}`))
	})

	p := NewGitHubIssuesPlugin()
	cfg := PluginConfig{Raw: `{"owner":"acme","repo":"widgets","token":"tok"}`}
	const comment = "Closed automatically — the linked backlog item was marked done in stapler-squad."
	err := p.PostIssueComment(context.Background(), cfg, "42", comment)
	require.NoError(t, err)

	require.Equal(t, http.MethodPost, gotMethod)
	require.Equal(t, "/repos/acme/widgets/issues/42/comments", gotPath)
	require.Equal(t, comment, gotBody["body"])
}

func TestGitHubPRsPlugin_PluginID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "github_prs", NewGitHubPRsPlugin().PluginID())
}

func TestGitHubPRsPlugin_Fetch_ReturnsEmptyWhenTokenMissing(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
