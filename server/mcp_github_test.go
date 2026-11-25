package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	githubpkg "github.com/tstapler/stapler-squad/github"
)

// ghRecorder serves canned bodies by REST path and records every path hit.
// Serial tests only: SetGhBaseURLForTest and GITHUB_TOKEN are process-global.
type ghRecorder struct {
	mu    sync.Mutex
	paths []string
}

func (r *ghRecorder) hits() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

func newGHRecorder(t *testing.T, bodies map[string]string) *ghRecorder {
	t.Helper()
	rec := &ghRecorder{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec.mu.Lock()
		rec.paths = append(rec.paths, req.URL.Path)
		rec.mu.Unlock()
		body, ok := bodies[req.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	t.Cleanup(githubpkg.SetGhBaseURLForTest(ts.URL + "/"))
	t.Setenv("GITHUB_TOKEN", "fake-token")
	return rec
}

func mustRepo(t *testing.T, owner, repo string) githubpkg.RepoRef {
	t.Helper()
	ref, err := githubpkg.NewRepoRef(owner, repo)
	require.NoError(t, err)
	return ref
}

func TestNewMCPGitHub_should_FetchIssueByNumber(t *testing.T) {
	rec := newGHRecorder(t, map[string]string{
		"/repos/o/r/issues/316": `{"number":316,"title":"bug","body":"b","state":"open","html_url":"https://github.com/o/r/issues/316"}`,
	})

	got, err := NewMCPGitHub().GetIssue(context.Background(), githubpkg.AccountRef{}, mustRepo(t, "o", "r"), 316)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 316, got.Number)
	assert.Equal(t, "bug", got.Title)
	assert.Equal(t, []string{"/repos/o/r/issues/316"}, rec.hits())
}

func TestNewMCPGitHub_should_FetchPRByNumber(t *testing.T) {
	rec := newGHRecorder(t, map[string]string{
		"/repos/o/r/pulls/272": `{"number":272,"title":"fix","state":"open","html_url":"https://github.com/o/r/pull/272"}`,
	})

	got, err := NewMCPGitHub().GetPR(context.Background(), githubpkg.AccountRef{}, mustRepo(t, "o", "r"), 272)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 272, got.Number)
	assert.Equal(t, []string{"/repos/o/r/pulls/272"}, rec.hits())
}

func TestNewMCPGitHub_should_FetchCommitBySHA(t *testing.T) {
	rec := newGHRecorder(t, map[string]string{
		"/repos/o/r/commits/abc123": `{"sha":"abc123","html_url":"https://github.com/o/r/commit/abc123","commit":{"message":"msg","author":{"name":"tstapler"}}}`,
	})

	got, err := NewMCPGitHub().GetCommit(context.Background(), githubpkg.AccountRef{}, mustRepo(t, "o", "r"), "abc123")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "abc123", got.SHA)
	assert.Equal(t, "msg", got.Message)
	assert.Equal(t, []string{"/repos/o/r/commits/abc123"}, rec.hits())
}

func TestNewMCPGitHub_should_FetchPRInfoByNumber(t *testing.T) {
	rec := newGHRecorder(t, map[string]string{
		"/repos/o/r/pulls/7": `{"number":7,"state":"open","html_url":"u","head":{"ref":"feat"},"base":{"ref":"main","repo":{"full_name":"o/r"}},"user":{"login":"tstapler"}}`,
	})

	got, err := NewMCPGitHub().GetPRByNumber(context.Background(), mustRepo(t, "o", "r"), 7)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "feat", got.HeadRef)
	assert.Equal(t, "tstapler", got.Author)
	assert.Equal(t, []string{"/repos/o/r/pulls/7"}, rec.hits())
}

// A GHE host has no github.com token: with GITHUB_TOKEN set, reaching the
// github package with the host intact yields ErrNotAuthenticated, while a
// dropped host would fetch from the github.com test server and succeed.
func TestNewMCPGitHub_should_PassAccountHost_ForGHEIssueCommitAndPR(t *testing.T) {
	rec := newGHRecorder(t, map[string]string{
		"/repos/o/r/issues/1":     `{"number":1}`,
		"/repos/o/r/pulls/1":      `{"number":1}`,
		"/repos/o/r/commits/abc1": `{"sha":"abc1"}`,
	})
	gh := NewMCPGitHub()
	acct := githubpkg.AccountRef{Host: "ghe.example.com"}
	repo := mustRepo(t, "o", "r")

	_, issueErr := gh.GetIssue(context.Background(), acct, repo, 1)
	_, prErr := gh.GetPR(context.Background(), acct, repo, 1)
	_, commitErr := gh.GetCommit(context.Background(), acct, repo, "abc1")

	assert.ErrorIs(t, issueErr, githubpkg.ErrNotAuthenticated)
	assert.ErrorIs(t, prErr, githubpkg.ErrNotAuthenticated)
	assert.ErrorIs(t, commitErr, githubpkg.ErrNotAuthenticated)
	assert.Empty(t, rec.hits(), "a GHE account must never be sent to the github.com API")
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNewMCPGitHub_should_RouteLoginAndPRInfoToGHEHost(t *testing.T) {
	var mu sync.Mutex
	var urls []string
	t.Cleanup(githubpkg.SetGHHTTPBaseTransportForTest(rtFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		urls = append(urls, r.URL.Host+r.URL.Path)
		mu.Unlock()
		body := `{"number":3,"state":"open","head":{"ref":"b"},"base":{"ref":"main","repo":{"full_name":"o/r"}},"user":{"login":"ghe-user"}}`
		if r.URL.Path == "/api/v3/user" {
			body = `{"login":"ghe-user"}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})))
	gh := NewMCPGitHub()
	repo, err := githubpkg.NewRepoRefWithHost("o", "r", "ghe.example.com")
	require.NoError(t, err)

	login, loginErr := gh.GetCurrentUserLogin(context.Background(), "ghe.example.com")
	info, infoErr := gh.GetPRByNumber(context.Background(), repo, 3)

	require.NoError(t, loginErr)
	require.NoError(t, infoErr)
	assert.Equal(t, "ghe-user", login)
	assert.Equal(t, "ghe-user", info.Author)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"ghe.example.com/api/v3/user", "ghe.example.com/api/v3/repos/o/r/pulls/3"}, urls)
}
