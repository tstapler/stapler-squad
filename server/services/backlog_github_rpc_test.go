package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	gh "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
)

// resetGhBaseURL overrides GhBaseURL for a test and returns a restore func.
func resetGhBaseURL(ts *httptest.Server) func() {
	return gh.SetGhBaseURLForTest(ts.URL + "/")
}

// fakeGitHubEnterpriseHandler serves both REST (GET /user) and GraphQL
// (POST /api/graphql) requests that UserPRCache.fetch fires when resolving
// and refreshing an account: /user login-checks, /api/graphql fetches open
// PRs. Without a GraphQL responder, a test that only mocks /user leaves the
// GraphQL call to fall through to whatever graphQLURLForHost resolves to —
// this is what regressed into the real, unmocked github.com dial that hung
// CI (see graphQLURLForHost's EnterpriseBaseURLOverride fix in
// github/hosts.go).
func fakeGitHubEnterpriseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if strings.Contains(r.URL.Path, "graphql") {
		_, _ = w.Write([]byte(`{"data":{"viewer":{"pullRequests":{"nodes":[]}}}}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"login": "octocat"})
}

// ─── SearchGitHubRepos ────────────────────────────────────────────────────────

func TestSearchGitHubRepos_NilStorage(t *testing.T) {
	t.Parallel()
	svc := newBacklogServiceNilStorage()
	_, err := svc.SearchGitHubRepos(context.Background(),
		connect.NewRequest(&sessionv1.SearchGitHubReposRequest{}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnavailable, connectErr.Code())
}

func TestSearchGitHubRepos_DefaultLimit(t *testing.T) {
	var capturedPerPage string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPerPage = r.URL.Query().Get("per_page")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	svc := newBacklogService(t)
	_, err := svc.SearchGitHubRepos(context.Background(),
		connect.NewRequest(&sessionv1.SearchGitHubReposRequest{Limit: 0}))
	require.NoError(t, err)
	assert.Equal(t, "30", capturedPerPage)
}

func TestSearchGitHubRepos_ReturnsRepos(t *testing.T) {
	repos := []map[string]any{
		{"name": "repo1", "owner": map[string]any{"login": "acme"}, "description": "first", "private": false, "pushed_at": "2024-01-01"},
		{"name": "repo2", "owner": map[string]any{"login": "acme"}, "description": "second", "private": true, "pushed_at": "2024-01-02"},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos)
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	svc := newBacklogService(t)
	resp, err := svc.SearchGitHubRepos(context.Background(),
		connect.NewRequest(&sessionv1.SearchGitHubReposRequest{Limit: 30}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Repos, 2)
	assert.Equal(t, "acme", resp.Msg.Repos[0].Owner)
	assert.Equal(t, "repo1", resp.Msg.Repos[0].Repo)
	assert.False(t, resp.Msg.Repos[0].IsLocal)
}

// ─── ListGitHubIssues ─────────────────────────────────────────────────────────

func TestListGitHubIssues_NilStorage(t *testing.T) {
	t.Parallel()
	svc := newBacklogServiceNilStorage()
	_, err := svc.ListGitHubIssues(context.Background(),
		connect.NewRequest(&sessionv1.ListGitHubIssuesRequest{Owner: "a", Repo: "b"}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnavailable, connectErr.Code())
}

func TestListGitHubIssues_EmptyOwner(t *testing.T) {
	t.Parallel()
	svc := newBacklogService(t)
	_, err := svc.ListGitHubIssues(context.Background(),
		connect.NewRequest(&sessionv1.ListGitHubIssuesRequest{Owner: "", Repo: "repo"}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestListGitHubIssues_EmptyRepo(t *testing.T) {
	t.Parallel()
	svc := newBacklogService(t)
	_, err := svc.ListGitHubIssues(context.Background(),
		connect.NewRequest(&sessionv1.ListGitHubIssuesRequest{Owner: "owner", Repo: ""}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestListGitHubIssues_InvalidOwnerChars(t *testing.T) {
	t.Parallel()
	svc := newBacklogService(t)
	_, err := svc.ListGitHubIssues(context.Background(),
		connect.NewRequest(&sessionv1.ListGitHubIssuesRequest{Owner: "a b", Repo: "repo"}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestListGitHubIssues_DefaultState(t *testing.T) {
	var capturedQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	svc := newBacklogService(t)
	_, err := svc.ListGitHubIssues(context.Background(),
		connect.NewRequest(&sessionv1.ListGitHubIssuesRequest{Owner: "tstapler", Repo: "stapler-squad", State: ""}))
	require.NoError(t, err)
	assert.Contains(t, capturedQuery, "state=open", "empty state should default to 'open'")
}

func TestListGitHubIssues_SearchUsesSearchAPI(t *testing.T) {
	var capturedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	svc := newBacklogService(t)
	_, err := svc.ListGitHubIssues(context.Background(),
		connect.NewRequest(&sessionv1.ListGitHubIssuesRequest{
			Owner:  "tstapler",
			Repo:   "stapler-squad",
			Search: "login bug",
		}))
	require.NoError(t, err)
	assert.Equal(t, "/search/issues", capturedPath)
}

func TestListGitHubIssues_PopulatesBodyAuthorAndTimestamps(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"number":     5,
				"title":      "Import picker does nothing",
				"body":       "Selecting an issue never creates a backlog item.",
				"state":      "open",
				"html_url":   "https://github.com/acme/widgets/issues/5",
				"user":       map[string]any{"login": "reporter42"},
				"created_at": "2026-07-01T12:00:00Z",
				"updated_at": "2026-07-02T12:00:00Z",
			},
		})
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	svc := newBacklogService(t)
	resp, err := svc.ListGitHubIssues(context.Background(),
		connect.NewRequest(&sessionv1.ListGitHubIssuesRequest{Owner: "acme", Repo: "widgets"}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Issues, 1)
	entry := resp.Msg.Issues[0]
	assert.Equal(t, "Selecting an issue never creates a backlog item.", entry.Body)
	assert.Equal(t, "reporter42", entry.Author)
	assert.False(t, entry.IsPr)
	require.NotNil(t, entry.CreatedAt)
	require.NotNil(t, entry.UpdatedAt)
	assert.Equal(t, "2026-07-01T12:00:00Z", entry.CreatedAt.AsTime().Format(time.RFC3339))
	assert.Equal(t, "2026-07-02T12:00:00Z", entry.UpdatedAt.AsTime().Format(time.RFC3339))
}

// ─── ImportGitHubIssue ────────────────────────────────────────────────────────

func TestImportGitHubIssue_NilStorage(t *testing.T) {
	t.Parallel()
	svc := newBacklogServiceNilStorage()
	_, err := svc.ImportGitHubIssue(context.Background(),
		connect.NewRequest(&sessionv1.ImportGitHubIssueRequest{IssueUrl: "https://github.com/acme/widgets/issues/1"}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnavailable, connectErr.Code())
}

func TestImportGitHubIssue_EmptyURL(t *testing.T) {
	t.Parallel()
	svc := newBacklogService(t)
	_, err := svc.ImportGitHubIssue(context.Background(),
		connect.NewRequest(&sessionv1.ImportGitHubIssueRequest{IssueUrl: ""}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestImportGitHubIssue_NotAnIssueURL(t *testing.T) {
	t.Parallel()
	svc := newBacklogService(t)
	_, err := svc.ImportGitHubIssue(context.Background(),
		connect.NewRequest(&sessionv1.ImportGitHubIssueRequest{IssueUrl: "https://github.com/acme/widgets"}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestImportGitHubIssue_FetchError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	svc := newBacklogService(t)
	_, err := svc.ImportGitHubIssue(context.Background(),
		connect.NewRequest(&sessionv1.ImportGitHubIssueRequest{IssueUrl: "https://github.com/acme/widgets/issues/7"}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}

func TestImportGitHubIssue_CreatesItemFromIssue(t *testing.T) {
	var capturedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number":   7,
			"title":    "Import picker does nothing",
			"body":     "Selecting an issue never creates a backlog item.",
			"state":    "open",
			"html_url": "https://github.com/acme/widgets/issues/7",
		})
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	svc := newBacklogService(t)
	resp, err := svc.ImportGitHubIssue(context.Background(),
		connect.NewRequest(&sessionv1.ImportGitHubIssueRequest{
			IssueUrl:     "https://github.com/acme/widgets/issues/7",
			RepoPath:     "/tmp/local-widgets",
			SkipPlanning: true,
		}))
	require.NoError(t, err)
	require.Equal(t, "/repos/acme/widgets/issues/7", capturedPath)
	require.NotNil(t, resp.Msg.Item)
	assert.Equal(t, "Import picker does nothing", resp.Msg.Item.Title)
	assert.Equal(t, "Selecting an issue never creates a backlog item.", resp.Msg.Item.Description)
	assert.Equal(t, "/tmp/local-widgets", resp.Msg.Item.RepoPath)
	assert.Equal(t, "idea", resp.Msg.Item.Status)
	assert.Contains(t, resp.Msg.Item.Notes, "https://github.com/acme/widgets/issues/7")
	assert.False(t, resp.Msg.TriageTriggered, "no headlessPool wired in test service, so triage should never fire")
}

// TestImportGitHubIssue_DedupsExistingImport guards the dedup check
// ImportGitHubIssue runs against GetBacklogItemByExternalURL before creating
// a new item: importing the same issue URL twice must return the
// already-created item (AlreadyExisted=true, same item ID) rather than
// creating a second backlog item or re-triggering triage.
func TestImportGitHubIssue_DedupsExistingImport(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number":   11,
			"title":    "Duplicate import guard missing",
			"body":     "Importing the same issue twice creates two backlog items.",
			"state":    "open",
			"html_url": "https://github.com/acme/widgets/issues/11",
		})
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	svc := newBacklogService(t)
	req := connect.NewRequest(&sessionv1.ImportGitHubIssueRequest{
		IssueUrl:     "https://github.com/acme/widgets/issues/11",
		RepoPath:     "/tmp/local-widgets",
		SkipPlanning: true,
	})

	first, err := svc.ImportGitHubIssue(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, first.Msg.Item)
	assert.False(t, first.Msg.AlreadyExisted, "first import should create a new item")
	firstID := first.Msg.Item.Id

	second, err := svc.ImportGitHubIssue(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, second.Msg.Item)
	assert.True(t, second.Msg.AlreadyExisted, "second import of the same issue URL should be flagged as a dup")
	assert.Equal(t, firstID, second.Msg.Item.Id, "second import should return the same item ID as the first")
	assert.False(t, second.Msg.TriageTriggered, "dedup path must not re-trigger triage")

	items, err := svc.storage.ListBacklogItems(context.Background(), session.BacklogItemFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1, "only one backlog item should exist after importing the same issue URL twice")
	assert.Equal(t, firstID, items[0].ID)
}

func TestImportGitHubIssue_ResolvesRepoPathWhenNotProvided(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number":   3,
			"title":    "Some issue",
			"body":     "Body text",
			"state":    "open",
			"html_url": "https://github.com/acme/widgets/issues/3",
		})
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	svc := newBacklogService(t)
	resolver := &fakeGitHubResolver{localPath: "/tmp/fake-clone/acme/widgets"}
	svc.SetGitHubResolver(resolver.resolve)

	resp, err := svc.ImportGitHubIssue(context.Background(),
		connect.NewRequest(&sessionv1.ImportGitHubIssueRequest{
			IssueUrl: "https://github.com/acme/widgets/issues/3",
		}))
	require.NoError(t, err)
	require.Len(t, resolver.calls, 1)
	assert.Equal(t, "acme/widgets", resolver.calls[0])
	assert.Equal(t, "/tmp/fake-clone/acme/widgets", resp.Msg.Item.RepoPath)
}

// ─── GetSessionBacklogIndex ───────────────────────────────────────────────────

func TestGetSessionBacklogIndex_NilStorage(t *testing.T) {
	t.Parallel()
	svc := newBacklogServiceNilStorage()
	_, err := svc.GetSessionBacklogIndex(context.Background(),
		connect.NewRequest(&sessionv1.GetSessionBacklogIndexRequest{}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnavailable, connectErr.Code())
}

func TestGetSessionBacklogIndex_EmptyDB(t *testing.T) {
	t.Parallel()
	svc := newBacklogService(t)
	resp, err := svc.GetSessionBacklogIndex(context.Background(),
		connect.NewRequest(&sessionv1.GetSessionBacklogIndexRequest{}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Entries)
}

func TestGetSessionBacklogIndex_WithEntries(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Test Backlog Item",
		Status: "idea",
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "test-session-uuid-abc",
		SessionRole: "work",
	})
	require.NoError(t, err)

	resp, err := svc.GetSessionBacklogIndex(context.Background(),
		connect.NewRequest(&sessionv1.GetSessionBacklogIndexRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Entries, 1)
	e := resp.Msg.Entries[0]
	assert.Equal(t, "test-session-uuid-abc", e.SessionUuid)
	assert.Equal(t, "Test Backlog Item", e.ItemTitle)
	assert.Equal(t, "idea", e.ItemStatus)
	assert.Equal(t, "work", e.SessionRole)
	assert.NotEmpty(t, e.ItemId)
}
