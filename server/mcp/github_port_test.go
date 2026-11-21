package mcp

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session/domain"
)

// httpGitHub is the real github-package client, for tests that point
// githubpkg.GhBaseURL at an httptest server.
type httpGitHub struct{}

func (httpGitHub) GetIssue(ctx context.Context, a githubpkg.AccountRef, r githubpkg.RepoRef, n int) (*githubpkg.IssueResult, error) {
	return githubpkg.GetIssue(ctx, a, r, n)
}

func (httpGitHub) GetPR(ctx context.Context, a githubpkg.AccountRef, r githubpkg.RepoRef, n int) (*githubpkg.PRResult, error) {
	return githubpkg.GetPR(ctx, a, r, n)
}

func (httpGitHub) GetPRByNumber(ctx context.Context, r githubpkg.RepoRef, n int) (*githubpkg.PRInfo, error) {
	return githubpkg.GetPRByNumber(ctx, r, n)
}

func (httpGitHub) GetCommit(ctx context.Context, a githubpkg.AccountRef, r githubpkg.RepoRef, sha string) (*githubpkg.CommitResult, error) {
	return githubpkg.GetCommit(ctx, a, r, sha)
}

func (httpGitHub) GetCurrentUserLogin(ctx context.Context, host string) (string, error) {
	return githubpkg.GetCurrentUserLogin(ctx, host)
}

// fakeGitHub answers from fields and records each call as "Method:arg".
type fakeGitHub struct {
	mu    sync.Mutex
	calls []string

	issue  *githubpkg.IssueResult
	pr     *githubpkg.PRResult
	info   *githubpkg.PRInfo
	commit *githubpkg.CommitResult
	login  string
	err    error
}

func (f *fakeGitHub) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeGitHub) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeGitHub) GetIssue(_ context.Context, a githubpkg.AccountRef, _ githubpkg.RepoRef, n int) (*githubpkg.IssueResult, error) {
	f.record("GetIssue:" + a.Host + ":" + strconv.Itoa(n))
	return f.issue, f.err
}

func (f *fakeGitHub) GetPR(_ context.Context, _ githubpkg.AccountRef, _ githubpkg.RepoRef, n int) (*githubpkg.PRResult, error) {
	f.record("GetPR:" + strconv.Itoa(n))
	return f.pr, f.err
}

func (f *fakeGitHub) GetPRByNumber(_ context.Context, _ githubpkg.RepoRef, n int) (*githubpkg.PRInfo, error) {
	f.record("GetPRByNumber:" + strconv.Itoa(n))
	return f.info, f.err
}

func (f *fakeGitHub) GetCommit(_ context.Context, _ githubpkg.AccountRef, _ githubpkg.RepoRef, sha string) (*githubpkg.CommitResult, error) {
	f.record("GetCommit:" + sha)
	return f.commit, f.err
}

func (f *fakeGitHub) GetCurrentUserLogin(_ context.Context, host string) (string, error) {
	f.record("GetCurrentUserLogin:" + host)
	return f.login, f.err
}

func TestGitHubFromOptions_should_CarryCallersPort(t *testing.T) {
	t.Parallel()
	assert.Nil(t, GitHubFromOptions())
	fake := &fakeGitHub{}
	assert.Same(t, fake, GitHubFromOptions(WithGitHub(fake)))
}

func TestNewCore_should_UseInjectedGitHub_ForImportGitHubIssue(t *testing.T) {
	t.Parallel()
	fake := &fakeGitHub{issue: &githubpkg.IssueResult{Number: 316, Title: "bug from fake", Body: "body", URL: "https://github.com/o/r/issues/316"}}
	s := NewCore(&stubStore{}, nil, nil, newTestBacklogStorage(t), nil, nil, nil, nil, nil, nil, WithGitHub(fake))

	res, err := s.GetTool("import_github_issue").Handler(context.Background(), makeToolReq(map[string]interface{}{"issue_url": "https://github.com/o/r/issues/316"}))

	require.NoError(t, err)
	assert.Contains(t, toolResultText(t, res), "bug from fake")
	assert.Equal(t, []string{"GetIssue:github.com:316"}, fake.recorded())
}

func TestNewCore_should_ReportUnavailable_When_GitHubAbsent(t *testing.T) {
	t.Parallel()
	_, call := newCoreWithStorage(t, &stubStore{})

	got := call("import_github_issue", map[string]interface{}{"issue_url": "https://github.com/o/r/issues/316"})

	assert.Equal(t, false, got["success"])
	assert.Equal(t, ErrUnavailable, resultErrField(got, "code"))
	assert.Contains(t, resultErrField(got, "message"), "GitHub access is not wired")
}

func TestVerifyGitHubRefExists_should_RouteEachRefTypeToItsPortMethod(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		url  string
		want string
	}{
		"pr":     {"https://github.com/o/r/pull/272", "GetPR:272"},
		"issue":  {"https://github.com/o/r/issues/9", "GetIssue:github.com:9"},
		"commit": {"https://github.com/o/r/commit/abc123", "GetCommit:abc123"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeGitHub{}
			ref, err := githubpkg.ParseGitHubRef(tc.url)
			require.NoError(t, err)

			require.NoError(t, (&backlogHandlers{github: fake}).verifyGitHubRefExists(context.Background(), ref))

			assert.Equal(t, []string{tc.want}, fake.recorded())
		})
	}
}

func TestVerifyGitHubRefExists_should_FailUnavailable_When_GitHubAbsent(t *testing.T) {
	t.Parallel()
	for _, url := range []string{"https://github.com/o/r/pull/1", "https://github.com/o/r/issues/1", "https://github.com/o/r/commit/abc123"} {
		ref, err := githubpkg.ParseGitHubRef(url)
		require.NoError(t, err)

		err = (&backlogHandlers{}).verifyGitHubRefExists(context.Background(), ref)

		assert.ErrorIs(t, err, ErrGitHubUnavailable, url)
	}
}

func TestCallerGitHubLogin_should_AskPortForHost(t *testing.T) {
	t.Parallel()
	fake := &fakeGitHub{login: "tstapler"}

	got, err := (&backlogHandlers{github: fake}).callerGitHubLogin(context.Background(), "ghe.example.com")

	require.NoError(t, err)
	assert.Equal(t, "tstapler", got)
	assert.Equal(t, []string{"GetCurrentUserLogin:ghe.example.com"}, fake.recorded())
}

func TestCallerGitHubLogin_should_FailUnavailable_When_GitHubAbsent(t *testing.T) {
	t.Parallel()
	_, err := (&backlogHandlers{}).callerGitHubLogin(context.Background(), "")
	assert.ErrorIs(t, err, ErrGitHubUnavailable)
}

func TestVerifyPR_should_UseInjectedPortAndMapResult(t *testing.T) {
	t.Parallel()
	fake := &fakeGitHub{info: &githubpkg.PRInfo{HeadRef: "feat", State: "OPEN", Author: "tstapler"}}
	h := &backlogHandlers{github: fake}

	v, err := h.verifyPR(context.Background(), mustRepoRef(t, "o", "r"), 7, "feat")

	require.NoError(t, err)
	assert.True(t, v.Exists)
	assert.True(t, v.Matched)
	assert.Equal(t, "tstapler", v.Author)
	assert.Equal(t, []string{"GetPRByNumber:7"}, fake.recorded())
}

func TestVerifyPR_should_ReportNoPR_When_PortReturnsErrNoPR(t *testing.T) {
	t.Parallel()
	fake := &fakeGitHub{err: githubpkg.ErrNoPR}

	v, err := (&backlogHandlers{github: fake}).verifyPR(context.Background(), mustRepoRef(t, "o", "r"), 7, "feat")

	require.NoError(t, err)
	assert.False(t, v.Exists)
}

func TestReportDuplicate_should_ReportPermanentUnavailable_When_GitHubAbsent(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, domain.BacklogStatusInProgress)
	h := &backlogHandlers{storage: storage}

	res, err := h.reportDuplicate(WithSessionUUID(context.Background(), sessionUUID), makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/o/r/pull/272",
		"reason":        "same fix already merged in that PR",
	}))

	require.NoError(t, err)
	got := parseResult(t, res)
	assert.Equal(t, ErrUnavailable, resultErrField(got, "code"))
	msg := resultErrField(got, "message")
	assert.Contains(t, msg, "not transient")
	assert.NotContains(t, msg, "retry:")
}

func TestVerifyPR_should_ReturnTransientError_When_GitHubAbsent(t *testing.T) {
	t.Parallel()
	v, err := (&backlogHandlers{}).verifyPR(context.Background(), mustRepoRef(t, "o", "r"), 7, "feat")

	assert.True(t, errors.Is(err, ErrGitHubUnavailable))
	assert.Equal(t, PRVerification{}, v)
}
