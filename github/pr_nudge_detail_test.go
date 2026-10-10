package github

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const nudgeDetailOK = `{"data":{"repository":{"pullRequest":{
"url":"https://x/acme/api/pull/42","state":"OPEN","isDraft":false,"headRefName":"fix","isCrossRepository":true,"mergeable":"CONFLICTING",
"commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
 {"__typename":"CheckRun","name":"lint","conclusion":"FAILURE","detailsUrl":"https://ci/lint"},
 {"__typename":"CheckRun","name":"ok","conclusion":"SUCCESS"},
 {"__typename":"StatusContext","context":"ci/legacy","state":"ERROR","targetUrl":"https://ci/legacy"}
]}}}}]},
"reviewThreads":{"totalCount":25,"nodes":[
 {"isResolved":false,"isOutdated":false,"comments":{"nodes":[{"author":{"login":"rev"},"url":"https://x/c1","path":"a.go"}]}},
 {"isResolved":true,"isOutdated":false,"comments":{"nodes":[{"author":{"login":"rev"},"url":"https://x/c2","path":"b.go"}]}},
 {"isResolved":false,"isOutdated":true,"comments":{"nodes":[{"author":{"login":"rev"},"url":"https://x/c3","path":"c.go"}]}},
 {"isResolved":false,"isOutdated":false,"comments":{"nodes":[{"author":null,"url":"https://x/c4","path":"d.go"}]}}
]}}}}}`

// authRecorder records each request's Authorization header and GraphQL query.
type authRecorder struct {
	mu      sync.Mutex
	auth    []string
	queries []string
	status  int
	body    string
	headers map[string]string
}

func (a *authRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Query string `json:"query"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	a.mu.Lock()
	a.auth = append(a.auth, r.Header.Get("Authorization"))
	a.queries = append(a.queries, in.Query)
	a.mu.Unlock()
	for k, v := range a.headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "application/json")
	status := a.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	body := a.body
	if body == "" {
		body = nudgeDetailOK
	}
	_, _ = w.Write([]byte(body))
}

func TestFetchPRNudgeDetail_should_SendSuppliedTokenAndNotLookupDefault_When_Github_And_GHEHosts(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "env-default-token")
	t.Setenv("GH_TOKEN", "env-default-token")
	ctx := t.Context()

	dotcom := &authRecorder{}
	startGitHubCom(t, dotcom)
	ghe := &authRecorder{}
	startGHE(t, "ghe.corp", ghe)

	d, err := FetchPRNudgeDetail(ctx, mustPRKey(t, "github.com", "acme", "api", 42), "tok-carol")
	require.NoError(t, err)
	require.Equal(t, []string{"Bearer tok-carol"}, dotcom.auth)

	_, err = FetchPRNudgeDetail(ctx, mustPRKey(t, "ghe.corp", "corp", "svc", 7), "tok-bob")
	require.NoError(t, err)
	require.Equal(t, []string{"Bearer tok-bob"}, ghe.auth)

	require.Equal(t, "open", d.State)
	require.Equal(t, "fix", d.HeadRef)
	require.True(t, d.IsCrossRepository)
	require.NotNil(t, d.HasMergeConflict)
	require.True(t, *d.HasMergeConflict)
	require.Equal(t, []FailingCheck{
		{Name: "lint", URL: "https://ci/lint", Conclusion: "failure"},
		{Name: "ci/legacy", URL: "https://ci/legacy", Conclusion: "error"},
	}, d.FailingChecks)
	require.Equal(t, []PRThreadRef{
		{AuthorLogin: "rev", Path: "a.go", URL: "https://x/c1"},
		{AuthorLogin: "", Path: "d.go", URL: "https://x/c4"},
	}, d.UnresolvedThreads)
	require.True(t, d.MoreThreadsUnseen, "totalCount 25 > 4 nodes read")
}

func TestFetchPRNudgeDetail_should_ReturnErrRateLimited_When_LimiterRejects(t *testing.T) {
	rec := &authRecorder{}
	startGitHubCom(t, rec)
	key := mustPRKey(t, "github.com", "acme", "api", 42)

	DefaultRateLimiter.setLimitedUntil(time.Now().Add(time.Minute))
	t.Cleanup(DefaultRateLimiter.Reset)
	_, err := FetchPRNudgeDetail(t.Context(), key, "tok")
	require.ErrorIs(t, err, ErrRateLimited)
	require.Empty(t, rec.auth, "no request while limited")
	DefaultRateLimiter.Reset()

	// Server-side limit signals map to the same typed error.
	for name, h := range map[string]*authRecorder{
		"429":          {status: http.StatusTooManyRequests},
		"403 retry":    {status: http.StatusForbidden, headers: map[string]string{"Retry-After": "1"}},
		"graphql type": {body: `{"errors":[{"type":"RATE_LIMITED","message":"API rate limit exceeded"}]}`},
	} {
		startGitHubCom(t, h)
		_, err := FetchPRNudgeDetail(t.Context(), key, "tok")
		require.ErrorIs(t, err, ErrRateLimited, name)
		DefaultRateLimiter.Reset()
	}
}

func TestFetchPRNudgeDetail_should_WrapNotFound_When_PRMissingOrDenied(t *testing.T) {
	key := mustPRKey(t, "github.com", "acme", "api", 42)
	for name, h := range map[string]*authRecorder{
		"null pr":   {body: `{"data":{"repository":{"pullRequest":null}},"errors":[{"type":"NOT_FOUND","message":"x"}]}`},
		"no repo":   {body: `{"data":{"repository":null}}`},
		"forbidden": {status: http.StatusForbidden},
		"401":       {status: http.StatusUnauthorized},
	} {
		startGitHubCom(t, h)
		_, err := FetchPRNudgeDetail(t.Context(), key, "tok")
		require.ErrorIs(t, err, ErrGitHubRefNotFound, name)
		require.False(t, errors.Is(err, ErrRateLimited), name)
	}
	_, err := FetchPRNudgeDetail(t.Context(), key, "")
	require.Error(t, err, "empty token is rejected, never defaulted")
}

func TestFetchPRNudgeDetail_should_NeverRequestCommentBody_When_BuildingQuery(t *testing.T) {
	require.NotContains(t, strings.ToLower(prNudgeDetailQuery), "body")

	rec := &authRecorder{}
	startGitHubCom(t, rec)
	_, err := FetchPRNudgeDetail(t.Context(), mustPRKey(t, "github.com", "acme", "api", 42), "tok")
	require.NoError(t, err)
	require.Len(t, rec.queries, 1)
	require.NotContains(t, strings.ToLower(rec.queries[0]), "body")
}

func TestFetchPRNudgeDetail_should_RequestOnlyAuthorLoginUrlPathForThreadComments_When_BuildingQuery(t *testing.T) {
	require.Contains(t, prNudgeDetailQuery, "comments(first:1){nodes{author{login} url path}}")

	// The decode target has no body-like field either.
	var walk func(rt reflect.Type)
	walk = func(rt reflect.Type) {
		for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			require.NotContains(t, strings.ToLower(f.Tag.Get("json")), "body", f.Name)
			require.NotContains(t, strings.ToLower(f.Name), "body", f.Name)
			walk(f.Type)
		}
	}
	walk(reflect.TypeOf(prNudgeNode{}))
	walk(reflect.TypeOf(PRNudgeDetail{}))
}

func TestFetchPRNudgeDetail_should_NotReportNotFound_When_ErrorsOnlyAreNotNotFound(t *testing.T) {
	key := mustPRKey(t, "github.com", "acme", "api", 42)
	for name, body := range map[string]string{
		"sso":    `{"data":{"repository":{"pullRequest":null}},"errors":[{"type":"FORBIDDEN","message":"Resource protected by organization SAML enforcement"}]}`,
		"scope":  `{"data":null,"errors":[{"type":"INSUFFICIENT_SCOPES","message":"missing scope"}]}`,
		"outage": `{"errors":[{"message":"Something went wrong"}]}`,
	} {
		startGitHubCom(t, &authRecorder{body: body})
		_, err := FetchPRNudgeDetail(t.Context(), key, "tok")
		require.Error(t, err, name)
		require.False(t, errors.Is(err, ErrGitHubRefNotFound), name)
		require.False(t, errors.Is(err, ErrRateLimited), name)
	}
}

func TestFetchPRNudgeDetail_should_FailInsteadOfOmitReasons_When_PartialResponseHasErrors(t *testing.T) {
	key := mustPRKey(t, "github.com", "acme", "api", 42)
	startGitHubCom(t, &authRecorder{body: `{"data":{"repository":{"pullRequest":{"url":"u","state":"OPEN","commits":{"nodes":[]},"reviewThreads":{"totalCount":0,"nodes":[]}}}},"errors":[{"type":"SERVICE_UNAVAILABLE","message":"threads timed out"}]}`})
	_, err := FetchPRNudgeDetail(t.Context(), key, "tok")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrGitHubRefNotFound))
}

func TestFetchPRNudgeDetail_should_FlagMoreChecksUnseen_When_ContextsExceedPage(t *testing.T) {
	key := mustPRKey(t, "github.com", "acme", "api", 42)
	body := strings.Replace(nudgeDetailOK, `"statusCheckRollup":{"contexts":{"nodes"`, `"statusCheckRollup":{"contexts":{"totalCount":130,"nodes"`, 1)
	startGitHubCom(t, &authRecorder{body: body})
	d, err := FetchPRNudgeDetail(t.Context(), key, "tok")
	require.NoError(t, err)
	require.True(t, d.MoreChecksUnseen)
}
