package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gh "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
	"github.com/zalando/go-keyring"
)

// resetGhBaseURL is defined once for the package in tools_backlog_test.go
// (it overrides githubpkg.GhBaseURL — same package as gh here, aliased
// differently per file) and reused here rather than redeclared.

// --- NewPRVerification (gap: plan.md Task 2.1 describes this behavior but
// names no dedicated test for it — see validation.md) ---

// TestNewPRVerification_should_ForceMatchedFalse_When_MatchedTrueButNotExists
// pins the Matched ⇒ Exists invariant NewPRVerification enforces at
// construction: an illegal matched=true/exists=false combination must be
// coerced to matched=false (logged, never panicking), while a legal
// construction passes through unchanged.
func TestNewPRVerification_should_ForceMatchedFalse_When_MatchedTrueButNotExists(t *testing.T) {
	t.Run("illegal construction is coerced", func(t *testing.T) {
		v := NewPRVerification(false, true, "some-branch", gh.PRStateOpen, "tstapler")
		assert.False(t, v.Exists)
		assert.False(t, v.Matched, "matched must be forced to false when exists=false")
		// Non-invariant fields are carried through unvalidated.
		assert.Equal(t, "some-branch", v.ActualHeadBranch)
		assert.Equal(t, gh.PRStateOpen, v.State)
		assert.Equal(t, "tstapler", v.Author)
	})

	t.Run("legal construction passes through unchanged", func(t *testing.T) {
		v := NewPRVerification(true, true, "some-branch", gh.PRStateMerged, "tstapler")
		assert.True(t, v.Exists)
		assert.True(t, v.Matched)
		assert.Equal(t, "some-branch", v.ActualHeadBranch)
		assert.Equal(t, gh.PRStateMerged, v.State)
		assert.Equal(t, "tstapler", v.Author)
	})

	t.Run("exists=true, matched=false is legal (the fallback-branch case) and passes through", func(t *testing.T) {
		v := NewPRVerification(true, false, "feature/y", gh.PRStateOpen, "tstapler")
		assert.True(t, v.Exists)
		assert.False(t, v.Matched)
	})
}

// --- VerifyPRMatchesBranch rewritten body (gap: plan.md Task 2.1 rewrites
// this function but Story 2's task list only updates its callers' seam
// literals, never exercises the real function through an HTTP stub — see
// validation.md) ---

// TestVerifyPRMatchesBranch_should_ReturnMatchedTrue_When_HeadBranchEqualsExpected
// exercises the real VerifyPRMatchesBranch -> GetPRByNumber REST round trip
// via an httptest.Server + GhBaseURL override, confirming the happy/fast
// path (Matched=true) and that Author is threaded through from the
// response's user.login field.
func TestVerifyPRMatchesBranch_should_ReturnMatchedTrue_When_HeadBranchEqualsExpected(t *testing.T) {
	body := map[string]any{
		"number":   326,
		"html_url": "https://github.com/tstapler/stapler-squad/pull/326",
		"state":    "closed",
		"merged":   true,
		"head": map[string]any{
			"ref": "feature/ci-status-diff-viewer",
		},
		"base": map[string]any{
			"ref": "main",
			"repo": map[string]any{
				"full_name": "tstapler/stapler-squad",
			},
		},
		"user": map[string]any{
			"login": "tstapler",
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	v, err := VerifyPRMatchesBranch(context.Background(), "tstapler", "stapler-squad", 326, "feature/ci-status-diff-viewer")
	require.NoError(t, err)
	assert.True(t, v.Exists)
	assert.True(t, v.Matched)
	assert.Equal(t, "feature/ci-status-diff-viewer", v.ActualHeadBranch)
	assert.Equal(t, gh.PRStateMerged, v.State)
	assert.Equal(t, "tstapler", v.Author)
}

// TestVerifyPRMatchesBranch_should_ReturnError_When_GetPRByNumberFails
// confirms (PRVerification{}, err) propagates unchanged from
// GetPRByNumber's error — the exact contract reportPRCreated's transient-
// error handling depends on.
func TestVerifyPRMatchesBranch_should_ReturnError_When_GetPRByNumberFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	v, err := VerifyPRMatchesBranch(context.Background(), "tstapler", "stapler-squad", 326, "feature/ci-status-diff-viewer")
	require.Error(t, err)
	assert.Equal(t, PRVerification{}, v)
}

// --- create_session_for_pr existing-session short-circuit (async-session-
// creation Epic 2.3, Story 2.3.2, Task 2.3.2c) ---

// seedUserPRCacheWithOnePR drives a real githubpkg.UserPRCache.Refresh()
// against an httptest-backed GitHub API (the /user login lookup and the
// GraphQL PR-list query fetchUserPRsForToken issues), then Annotate()s the
// resulting snapshot with sessionTitle as the PR's associated session --
// producing exactly the cache state createSessionForPR's short-circuit
// checks (pr.Owner/pr.Repo/pr.Number match + len(pr.SessionIDs) > 0),
// without a mock/fake UserPRCache (there is no seam for one — cache is a
// concrete *githubpkg.UserPRCache field).
func seedUserPRCacheWithOnePR(t *testing.T, owner, repo string, prNumber int, branch, sessionTitle string) *gh.UserPRCache {
	t.Helper()

	// Isolate from any real GitHub account(s) configured in the OS keychain on
	// the machine running this test: collectAllTokens() (github/user_pr_cache.go)
	// reads GetAllKeychainTokens() unconditionally alongside GITHUB_TOKEN/GH_TOKEN,
	// and a real account's token would fetch real PRs from its own (non-overridden)
	// enterprise host, polluting this fixture's exactly-one-PR assumption -- see
	// github/http_client_test.go's identical keyring.MockInit() usage.
	keyring.MockInit()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "tstapler"})
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"viewer": map[string]any{
						"pullRequests": map[string]any{
							"nodes": []map[string]any{
								{
									"number":      prNumber,
									"title":       "test PR",
									"url":         "https://github.com/" + owner + "/" + repo + "/pull/1",
									"headRefName": branch,
									"baseRefName": "main",
									"state":       "OPEN",
									"isDraft":     false,
									"updatedAt":   "2026-01-01T00:00:00Z",
									"repository": map[string]any{
										"owner": map[string]any{"login": owner},
										"name":  repo,
									},
								},
							},
						},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)
	t.Cleanup(resetGhBaseURL(ts))
	t.Setenv("GITHUB_TOKEN", "fake-token")

	// Start (not a bare Refresh off an unstarted cache -- c.ctx is nil until
	// Start runs, and resolveAllLogins dereferences it) triggers loop()'s own
	// unconditional initial fetch. Wait for that to land rather than also
	// calling Refresh(): a second, independent fetch racing loop()'s could
	// complete *after* Annotate() below and silently overwrite the annotated
	// snapshot with a fresh, un-annotated one -- the default PollInterval
	// (2 minutes) is comfortably longer than this whole test, so once the
	// initial fetch is observed, no further background fetch will race
	// Annotate().
	cache := gh.NewUserPRCache()
	cache.Start(context.Background())
	t.Cleanup(cache.Stop)
	require.Eventually(t, func() bool { return len(cache.GetAll()) == 1 }, 5*time.Second, 10*time.Millisecond,
		"fixture setup: initial fetch never populated exactly one PR from the mocked GraphQL response")

	repoRef, err := gh.NewRepoRef(owner, repo)
	require.NoError(t, err)
	cache.Annotate([]gh.PRAnnotationSession{
		{ID: sessionTitle, Branch: branch, Repo: repoRef, PRNumber: prNumber},
	}, nil)

	prs := cache.GetAll()
	require.Len(t, prs, 1)
	require.Equal(t, []string{sessionTitle}, prs[0].SessionIDs, "fixture setup: Annotate must have attached the session ID")

	return cache
}

// TestCreateSessionForPR_should_ReturnExistingSession_When_PRAlreadyHasOne
// pins Story 2.3.2's acceptance criterion: a PR that already has an
// associated session must short-circuit before create_session_for_pr ever
// reaches CreateSession -- the handler is constructed with svc: nil (which
// createSessionForPR's post-short-circuit code path treats as "unavailable,"
// see the "no SessionService wired" error branch below the short-circuit),
// so if the short-circuit is bypassed the test observes that distinct error
// rather than the short-circuit's own success result.
func TestCreateSessionForPR_should_ReturnExistingSession_When_PRAlreadyHasOne(t *testing.T) {
	// Not t.Parallel(): seedUserPRCacheWithOnePR uses t.Setenv, which forbids it.
	const (
		owner        = "tstapler"
		repo         = "stapler-squad"
		branch       = "feature/existing-pr-session"
		prNumber     = 42
		sessionTitle = "existing-pr-session-title"
	)
	cache := seedUserPRCacheWithOnePR(t, owner, repo, prNumber, branch, sessionTitle)

	store := &stubStore{instances: []*session.Instance{
		{Title: sessionTitle, Path: t.TempDir(), Status: session.Active, Program: "claude"},
	}}
	ghHandlers := &githubHandlers{cache: cache, store: store, svc: nil}

	res, err := ghHandlers.createSessionForPR(context.Background(), makeToolReq(map[string]interface{}{
		"owner":     owner,
		"repo":      repo,
		"branch":    branch,
		"pr_number": float64(prNumber),
	}))
	require.NoError(t, err)

	m := parseResult(t, res)
	require.True(t, m["success"].(bool), "expected the short-circuit's success result, got: %+v", m)
	sessionField, ok := m["session"].(map[string]interface{})
	require.True(t, ok, "expected a session in the short-circuit result")
	assert.Equal(t, sessionTitle, sessionField["id"])
	assert.Nil(t, m["still_creating"], "the short-circuit path must never set still_creating")
}
