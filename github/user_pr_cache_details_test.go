package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// gqlRecorder is an httptest GraphQL endpoint that replies per query shape
// and records which shape each request used.
type gqlRecorder struct {
	mu       sync.Mutex
	queries  []string
	respond  func(query string) (status int, body string)
	requests int
}

func (g *gqlRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Query string `json:"query"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	g.mu.Lock()
	g.queries = append(g.queries, in.Query)
	g.requests++
	g.mu.Unlock()
	status, body := g.respond(in.Query)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func (g *gqlRecorder) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.requests
}

func newTestCache() *UserPRCache {
	c := NewUserPRCache()
	c.ctx = context.Background()
	return c
}

func startGitHubCom(t *testing.T, h http.Handler) {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(SetGhBaseURLForTest(ts.URL + "/"))
}

func startGHE(t *testing.T, host string, h http.Handler) {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	SetEnterpriseBaseURLOverride(host, ts.URL+"/")
	t.Cleanup(func() { SetEnterpriseBaseURLOverride(host, "") })
}

// prNode builds one GraphQL PR node; extra is spliced in after the base fields.
func prNode(number int, extra string) string {
	return fmt.Sprintf(`{"number":%d,"title":"t%d","url":"https://x/pull/%d","headRefName":"h","baseRefName":"main","state":"OPEN","isDraft":false,"updatedAt":"2026-01-01T00:00:00Z","closedAt":"","mergedAt":"","repository":{"owner":{"login":"acme"},"name":"api"},"reviewDecision":"","reviews":{"nodes":[]}%s}`, number, number, number, extra)
}

func prResponse(nodes ...string) string {
	return `{"data":{"viewer":{"pullRequests":{"nodes":[` + strings.Join(nodes, ",") + `]}}}}`
}

func commitWithContexts(contexts string) string {
	return `,"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[` + contexts + `]}}}}]}`
}

func fetchOne(t *testing.T, c *UserPRCache, host string) []UserPR {
	t.Helper()
	prs, err := c.fetchUserPRsForToken(host, "alice", "tok")
	if err != nil {
		t.Fatalf("fetchUserPRsForToken: %v", err)
	}
	return prs
}

func fixedResponse(body string) *gqlRecorder {
	return &gqlRecorder{respond: func(string) (int, string) { return http.StatusOK, body }}
}

func TestFetchUserPRsForToken_should_MapFailingChecksCappedAtTenCheckRunsFirst_When_12CheckRunsAndStatusContext(t *testing.T) {
	var ctxs []string
	ctxs = append(ctxs, `{"__typename":"StatusContext","context":"ci/legacy","state":"ERROR","targetUrl":"https://legacy"}`)
	ctxs = append(ctxs, `{"__typename":"CheckRun","name":"lint","conclusion":"FAILURE","detailsUrl":"https://lint"}`)
	for i := 1; i <= 11; i++ {
		ctxs = append(ctxs, fmt.Sprintf(`{"__typename":"CheckRun","name":"unit-%d","conclusion":"FAILURE","detailsUrl":"u"}`, i))
	}
	ctxs = append(ctxs, `{"__typename":"CheckRun","name":"ok","conclusion":"SUCCESS"}`,
		`{"__typename":"CheckRun","name":"running","conclusion":null}`,
		`{"__typename":"StatusContext","context":"ci/pending","state":"PENDING"}`)
	startGitHubCom(t, fixedResponse(prResponse(prNode(1, commitWithContexts(strings.Join(ctxs, ","))))))

	prs := fetchOne(t, newTestCache(), "github.com")
	got := prs[0].FailingChecks
	if len(got) != 10 {
		t.Fatalf("want 10 failing checks, got %d", len(got))
	}
	if got[0].Name != "lint" || got[0].URL != "https://lint" {
		t.Fatalf("lint must be first, got %+v", got[0])
	}
	for _, f := range got {
		if f.Name == "ok" || f.Name == "running" || f.Name == "ci/pending" || f.Name == "ci/legacy" {
			t.Fatalf("unexpected entry %q (legacy context sorts after 12 CheckRuns, cap is 10)", f.Name)
		}
	}
}

func TestFetchUserPRsForToken_should_CountOnlyUnresolvedNonOutdatedThreads_When_MixedThreads(t *testing.T) {
	threads := `,"reviewThreads":{"totalCount":3,"nodes":[{"isResolved":false,"isOutdated":false},{"isResolved":true,"isOutdated":false},{"isResolved":false,"isOutdated":true}]}`
	startGitHubCom(t, fixedResponse(prResponse(prNode(1, threads))))

	pr := fetchOne(t, newTestCache(), "github.com")[0]
	if pr.UnresolvedThreadCount == nil || *pr.UnresolvedThreadCount != 1 {
		t.Fatalf("want count 1, got %v", pr.UnresolvedThreadCount)
	}
	if !pr.DetailsLoaded || pr.UnresolvedThreadsTruncated {
		t.Fatalf("DetailsLoaded=%v truncated=%v", pr.DetailsLoaded, pr.UnresolvedThreadsTruncated)
	}
}

func TestFetchUserPRsForToken_should_MapMergeableToTriState_When_ConflictingMergeableUnknown(t *testing.T) {
	startGitHubCom(t, fixedResponse(prResponse(
		prNode(1, `,"mergeable":"CONFLICTING"`),
		prNode(2, `,"mergeable":"MERGEABLE"`),
		prNode(3, `,"mergeable":"UNKNOWN"`),
	)))
	prs := fetchOne(t, newTestCache(), "github.com")
	if prs[0].HasMergeConflict == nil || !*prs[0].HasMergeConflict {
		t.Fatalf("CONFLICTING must be *true, got %v", prs[0].HasMergeConflict)
	}
	if prs[1].HasMergeConflict == nil || *prs[1].HasMergeConflict {
		t.Fatalf("MERGEABLE must be *false, got %v", prs[1].HasMergeConflict)
	}
	if prs[2].HasMergeConflict != nil {
		t.Fatalf("UNKNOWN must be nil, got %v", *prs[2].HasMergeConflict)
	}
}

func TestFetchUserPRsForToken_should_CapAt50AndMarkTruncated_When_TotalThreadsOver50(t *testing.T) {
	nodes := strings.TrimSuffix(strings.Repeat(`{"isResolved":false,"isOutdated":false},`, 50), ",")
	threads := `,"reviewThreads":{"totalCount":73,"nodes":[` + nodes + `]}`
	startGitHubCom(t, fixedResponse(prResponse(prNode(1, threads))))

	pr := fetchOne(t, newTestCache(), "github.com")[0]
	if pr.UnresolvedThreadCount == nil || *pr.UnresolvedThreadCount != 50 || !pr.UnresolvedThreadsTruncated {
		t.Fatalf("want 50 + truncated, got %v truncated=%v", pr.UnresolvedThreadCount, pr.UnresolvedThreadsTruncated)
	}
}

const validationErrorsOnly = `{"errors":[{"type":"GRAPHQL_VALIDATION_FAILED","message":"Field 'reviewThreads' doesn't exist"}]}`

// widenedRejectingServer rejects any query containing reviewThreads or
// contexts, and serves the legacy shape otherwise.
func widenedRejectingServer(rejectBody string) *gqlRecorder {
	return &gqlRecorder{respond: func(q string) (int, string) {
		if strings.Contains(q, "reviewThreads") {
			return http.StatusOK, rejectBody
		}
		return http.StatusOK, prResponse(prNode(1, ""))
	}}
}

func TestFetchUserPRsForToken_should_RetryOnceWithLegacyQueryAndReturnPRsWithDetailsLoadedFalse_When_ErrorsOnlyValidationResponse(t *testing.T) {
	rec := widenedRejectingServer(validationErrorsOnly)
	startGitHubCom(t, rec)

	prs := fetchOne(t, newTestCache(), "github.com")
	if rec.count() != 2 {
		t.Fatalf("want exactly 2 requests, got %d", rec.count())
	}
	if rec.queries[1] != userPRGraphQLQueryLegacy {
		t.Fatalf("second request must use the legacy query")
	}
	if len(prs) != 1 || prs[0].DetailsLoaded || prs[0].UnresolvedThreadCount != nil {
		t.Fatalf("want 1 PR with details unloaded, got %+v", prs)
	}
}

func TestFetchUserPRsForToken_should_RetryOnceWithLegacyQuery_When_NodeLimitErrorResponse(t *testing.T) {
	rec := widenedRejectingServer(`{"errors":[{"type":"MAX_NODE_LIMIT_EXCEEDED","message":"This query could return more than 500000 nodes"}]}`)
	startGitHubCom(t, rec)
	if prs := fetchOne(t, newTestCache(), "github.com"); len(prs) != 1 {
		t.Fatalf("want 1 PR, got %d", len(prs))
	}
	if rec.count() != 2 {
		t.Fatalf("want 2 requests, got %d", rec.count())
	}

	// A second errors-only response on the legacy query is a real error, not a retry loop.
	always := &gqlRecorder{respond: func(string) (int, string) { return http.StatusOK, validationErrorsOnly }}
	startGitHubCom(t, always)
	if _, err := newTestCache().fetchUserPRsForToken("github.com", "alice", "tok"); err == nil {
		t.Fatal("want error when the legacy query is also rejected")
	}
	if always.count() != 2 {
		t.Fatalf("want exactly 2 requests (no retry loop), got %d", always.count())
	}
}

func TestFetchUserPRsForToken_should_SkipWidenedQueryOnLaterPollsForThatHostOnly_When_CapabilityCached(t *testing.T) {
	old := widenedRejectingServer(validationErrorsOnly)
	startGHE(t, "old.ghe", old)
	modern := fixedResponse(prResponse(prNode(1, `,"reviewThreads":{"totalCount":0,"nodes":[]}`)))
	startGHE(t, "new.ghe", modern)

	c := newTestCache()
	fetchOne(t, c, "old.ghe")
	fetchOne(t, c, "old.ghe")
	if old.count() != 3 {
		t.Fatalf("old host: want 2 requests on poll 1 + 1 on poll 2 = 3, got %d", old.count())
	}
	if old.queries[2] != userPRGraphQLQueryLegacy {
		t.Fatal("poll 2 must go straight to the legacy query")
	}
	prs := fetchOne(t, c, "new.ghe")
	if modern.count() != 1 || modern.queries[0] != userPRGraphQLQuery || !prs[0].DetailsLoaded {
		t.Fatalf("new host must be unaffected: requests=%d", modern.count())
	}
}

func TestFetchUserPRsForToken_should_NotRetryAndKeepPartialData_When_DataPresentWithNonFatalErrors(t *testing.T) {
	body := `{"data":{"viewer":{"pullRequests":{"nodes":[` + prNode(1, "") + `]}}},"errors":[{"type":"FORBIDDEN","message":"one node hidden"}]}`
	rec := fixedResponse(body)
	startGitHubCom(t, rec)
	prs := fetchOne(t, newTestCache(), "github.com")
	if rec.count() != 1 || len(prs) != 1 {
		t.Fatalf("want 1 request and 1 PR, got %d requests, %d PRs", rec.count(), len(prs))
	}
}

func TestFetchUserPRsForToken_should_ReturnPartialPRWithoutError_When_GHEResponseHasNoReviewThreads(t *testing.T) {
	startGHE(t, "ghe.corp", fixedResponse(prResponse(prNode(1, ""))))
	prs := fetchOne(t, newTestCache(), "ghe.corp")
	if len(prs) != 1 || prs[0].DetailsLoaded || prs[0].UnresolvedThreadCount != nil {
		t.Fatalf("want partial PR, got %+v", prs)
	}
}

func TestFetchUserPRsForToken_should_OmitThreadCountAndKeepChecks_When_DegradedModeFlagSet(t *testing.T) {
	rec := fixedResponse(prResponse(prNode(1, `,"mergeable":"CONFLICTING"`+commitWithContexts(`{"__typename":"CheckRun","name":"lint","conclusion":"FAILURE"}`))))
	startGitHubCom(t, rec)

	c := newTestCache()
	c.config.DegradedDetails = true
	pr := fetchOne(t, c, "github.com")[0]
	if strings.Contains(rec.queries[0], "reviewThreads") {
		t.Fatal("degraded query must not request reviewThreads")
	}
	if pr.UnresolvedThreadCount != nil || pr.DetailsLoaded {
		t.Fatalf("threads must be unknown in degraded mode: %+v", pr)
	}
	if len(pr.FailingChecks) != 1 || pr.HasMergeConflict == nil || !*pr.HasMergeConflict {
		t.Fatalf("checks and mergeable must survive: %+v", pr)
	}
}

func TestFetchUserPRsForToken_should_SetHostAndAccountLogin_When_TwoAccountsOnTwoHosts(t *testing.T) {
	startGitHubCom(t, fixedResponse(prResponse(prNode(1, ""))))
	startGHE(t, "ghe.corp", fixedResponse(prResponse(prNode(2, ""))))

	c := newTestCache()
	cases := []struct{ host, login string }{
		{"github.com", "alice"}, {"github.com", "carol"}, {"ghe.corp", "bob"}, {"ghe.corp", "dave"},
	}
	for _, tc := range cases {
		prs, err := c.fetchUserPRsForToken(tc.host, tc.login, "tok")
		if err != nil || len(prs) != 1 {
			t.Fatalf("%s/%s: prs=%d err=%v", tc.host, tc.login, len(prs), err)
		}
		if prs[0].Host != tc.host || prs[0].AccountLogin != tc.login {
			t.Fatalf("%s/%s: got host=%q login=%q", tc.host, tc.login, prs[0].Host, prs[0].AccountLogin)
		}
	}
}

// pollAccounts runs the poll's fan-out/merge via fetch() with pre-resolved accounts.
func pollAccounts(t *testing.T, c *UserPRCache, accounts []connectedAccount) {
	t.Helper()
	c.multiLogin.Store(&multiLoginState{accounts: accounts, checkedAt: time.Now()})
	if err := c.fetch(); err != nil {
		t.Fatalf("fetch: %v", err)
	}
}

func TestFetchUserPRsForToken_should_IssueOneGraphQLRequestPerAccountHostPerPoll_When_WidenedQuery(t *testing.T) {
	com := fixedResponse(prResponse(prNode(1, `,"reviewThreads":{"totalCount":0,"nodes":[]}`), prNode(2, "")))
	ghe := fixedResponse(prResponse(prNode(3, `,"reviewThreads":{"totalCount":0,"nodes":[]}`)))
	startGitHubCom(t, com)
	startGHE(t, "ghe.corp", ghe)

	pollAccounts(t, newTestCache(), []connectedAccount{
		{token: "t1", login: "alice", host: "github.com"},
		{token: "t2", login: "carol", host: "github.com"},
		{token: "t3", login: "bob", host: "ghe.corp"},
		{token: "t4", login: "dave", host: "ghe.corp"},
	})
	if got := com.count() + ghe.count(); got != 4 {
		t.Fatalf("want exactly 4 requests (one per account/host), got %d", got)
	}
}

func TestFetchUserPRs_should_ReportAccountStatusUnauthorizedRateLimitedErrorAndKeepOtherAccountsPRs_When_OneAccountFails(t *testing.T) {
	startGitHubCom(t, fixedResponse(prResponse(prNode(1, ""))))
	secret := "ghp_SECRETTOKEN"
	startGHE(t, "ghe.corp", &gqlRecorder{respond: func(string) (int, string) {
		return http.StatusUnauthorized, `{"message":"Bad credentials ` + secret + `"}`
	}})
	limited := &gqlRecorder{respond: func(string) (int, string) { return http.StatusTooManyRequests, `{}` }}
	startGHE(t, "limited.ghe", limited)
	startGHE(t, "broken.ghe", &gqlRecorder{respond: func(string) (int, string) {
		return http.StatusInternalServerError, "boom " + secret
	}})

	c := newTestCache()
	pollAccounts(t, c, []connectedAccount{
		{token: secret, login: "alice", host: "github.com"},
		{token: secret, login: "bob", host: "ghe.corp"},
		{token: secret, login: "rita", host: "limited.ghe"},
		{token: secret, login: "eve", host: "broken.ghe"},
	})

	if prs := c.GetAll(); len(prs) != 1 || prs[0].AccountLogin != "alice" {
		t.Fatalf("alice's PRs must survive other accounts' failures: %+v", prs)
	}
	want := []AccountPollStatus{
		{Host: "github.com", AccountLogin: "alice", State: AccountPollOK},
		{Host: "ghe.corp", AccountLogin: "bob", State: AccountPollUnauthorized},
		{Host: "limited.ghe", AccountLogin: "rita", State: AccountPollRateLimited},
		{Host: "broken.ghe", AccountLogin: "eve", State: AccountPollError},
	}
	got := c.AccountStatuses()
	if len(got) != len(want) {
		t.Fatalf("want %d statuses, got %+v", len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Host != w.Host || g.AccountLogin != w.AccountLogin || g.State != w.State {
			t.Fatalf("status %d: want %+v got %+v", i, w, g)
		}
		if strings.Contains(g.Detail, secret) {
			t.Fatalf("detail leaks token: %q", g.Detail)
		}
		if w.State != AccountPollOK && g.Detail == "" {
			t.Fatalf("status %d: failure needs a detail", i)
		}
	}
}
