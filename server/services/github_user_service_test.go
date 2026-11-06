package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	gh "github.com/tstapler/stapler-squad/github"
)

func newTestGitHubUserService(t *testing.T) *GitHubUserService {
	t.Helper()
	keyring.MockInit()
	cache := gh.NewUserPRCache()
	cache.Start(context.Background())
	t.Cleanup(cache.Stop)
	return NewGitHubUserService(cache, nil)
}

func TestAddGitHubAccountWithToken_ValidToken_StoresAndReturnsAccount(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"login": "octocat"})
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()

	svc := newTestGitHubUserService(t)

	resp, err := svc.AddGitHubAccountWithToken(context.Background(),
		connect.NewRequest(&sessionv1.AddGitHubAccountWithTokenRequest{Token: "test-token"}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.AuthState)
	assert.True(t, resp.Msg.AuthState.Available)
	assert.Equal(t, "octocat", resp.Msg.AuthState.Username)

	tok := gh.GetKeychainTokenForAccount("", "octocat")
	assert.Equal(t, "test-token", tok)
}

func TestAddGitHubAccountWithToken_InvalidToken_ReturnsPermissionDenied(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()

	svc := newTestGitHubUserService(t)

	_, err := svc.AddGitHubAccountWithToken(context.Background(),
		connect.NewRequest(&sessionv1.AddGitHubAccountWithTokenRequest{Token: "bad-token"}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodePermissionDenied, connectErr.Code())

	assert.Empty(t, gh.GetAllKeychainTokens())
}

func TestAddGitHubAccountWithToken_EmptyToken_ReturnsInvalidArgument(t *testing.T) {
	svc := newTestGitHubUserService(t)

	_, err := svc.AddGitHubAccountWithToken(context.Background(),
		connect.NewRequest(&sessionv1.AddGitHubAccountWithTokenRequest{Token: "   "}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestListGitHubAccounts_AccountOnUnconfiguredEnterpriseHost_IncludesHostInEnterpriseHosts(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(fakeGitHubEnterpriseHandler))
	defer ts.Close()
	const enterpriseHost = "github.netflix.net"
	gh.SetEnterpriseBaseURLOverride(enterpriseHost, ts.URL+"/")
	defer gh.SetEnterpriseBaseURLOverride(enterpriseHost, "")

	// svc has no statically configured enterprise hosts (newTestGitHubUserService
	// passes nil), mirroring an account added via AddGitHubAccountFromCLI/
	// AddGitHubAccountWithToken for a host with no OAuth App registered in
	// config.json.
	svc := newTestGitHubUserService(t)

	_, err := svc.AddGitHubAccountWithToken(context.Background(),
		connect.NewRequest(&sessionv1.AddGitHubAccountWithTokenRequest{
			Host:  enterpriseHost,
			Token: "test-token",
		}))
	require.NoError(t, err)

	resp, err := svc.ListGitHubAccounts(context.Background(),
		connect.NewRequest(&sessionv1.ListGitHubAccountsRequest{}))
	require.NoError(t, err)
	assert.Contains(t, resp.Msg.EnterpriseHosts, enterpriseHost)
}

// TestListGitHubAccounts_LinkedHostUnreachable_StillIncludesHostInEnterpriseHosts
// guards against a regression where EnterpriseHosts was derived from
// s.cache.GetCachedAccounts(), which reflects UserPRCache's live,
// network-dependent login revalidation (resolveAllLogins) rather than the
// account's persisted keychain entry. That revalidation silently drops an
// account on any transient failure to reach its host (see resolveAllLogins's
// "r.err != nil || r.acc.login == \"\"" skip) — very plausible for a
// VPN/corp-network-gated GHES host — which made the omnibar's GHE URL
// detector flicker in and out for a host the user had genuinely linked.
// EnterpriseHosts must instead come from ListKeychainAccounts, which is a
// local, non-network read of the persisted account index.
func TestListGitHubAccounts_LinkedHostUnreachable_StillIncludesHostInEnterpriseHosts(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(fakeGitHubEnterpriseHandler))
	const enterpriseHost = "github.netflix.net"
	gh.SetEnterpriseBaseURLOverride(enterpriseHost, ts.URL+"/")
	defer gh.SetEnterpriseBaseURLOverride(enterpriseHost, "")

	svc := newTestGitHubUserService(t)

	_, err := svc.AddGitHubAccountWithToken(context.Background(),
		connect.NewRequest(&sessionv1.AddGitHubAccountWithTokenRequest{
			Host:  enterpriseHost,
			Token: "test-token",
		}))
	require.NoError(t, err)

	// Simulate the host becoming unreachable (VPN drop, DNS hiccup, etc.) and
	// force a revalidation against it.
	ts.Close()
	svc.cache.InvalidateLoginCache()
	_ = svc.cache.Refresh(context.Background())
	require.Empty(t, svc.cache.GetCachedAccounts(),
		"test setup: revalidation against the closed server should have dropped the account from the live cache")

	resp, err := svc.ListGitHubAccounts(context.Background(),
		connect.NewRequest(&sessionv1.ListGitHubAccountsRequest{}))
	require.NoError(t, err)
	assert.Contains(t, resp.Msg.EnterpriseHosts, enterpriseHost,
		"EnterpriseHosts must stay derived from the persisted keychain account, not the live-revalidation cache")
}

func TestToProtoUserPR_should_SetUnresolvedThreadsTruncatedField25_When_GoUserPRTruncated(t *testing.T) {
	out := userPRsToProto([]gh.UserPR{
		{Number: 1, UnresolvedThreadCount: ptrTo(50), UnresolvedThreadsTruncated: true},
		{Number: 2, UnresolvedThreadCount: ptrTo(3)},
		{Number: 3},
	})
	if !out[0].GetUnresolvedThreadsTruncated() || out[0].GetUnresolvedThreadCount() != 50 {
		t.Fatalf("capped PR: %+v", out[0])
	}
	if out[1].GetUnresolvedThreadsTruncated() || out[1].UnresolvedThreadCount == nil || *out[1].UnresolvedThreadCount != 3 {
		t.Fatalf("under-cap PR: %+v", out[1])
	}
	if out[2].UnresolvedThreadCount != nil {
		t.Fatalf("absent count must stay unset, got %d", *out[2].UnresolvedThreadCount)
	}
}

func TestToProtoUserPR_should_LeaveThreadCountUnsetAndDefaultHost_When_DetailsNotLoaded(t *testing.T) {
	out := userPRsToProto([]gh.UserPR{{Number: 1, DetailsLoaded: false}})[0]
	if out.UnresolvedThreadCount != nil || out.GetDetailsLoaded() {
		t.Fatalf("count must be unset and details not loaded: %+v", out)
	}
	if out.GetHost() != "github.com" {
		t.Fatalf("empty host must default to github.com, got %q", out.GetHost())
	}
}

func TestToProtoUserPR_should_PopulateHostAccountLoginAndAllNewFieldsAt18To25_When_FullUserPR(t *testing.T) {
	out := userPRsToProto([]gh.UserPR{{
		Number:                     7,
		Host:                       "ghe.corp",
		AccountLogin:               "bob",
		FailingChecks:              []gh.FailingCheck{{Name: "lint", URL: "https://l", Conclusion: "failure"}},
		UnresolvedThreadCount:      ptrTo(50),
		UnresolvedThreadsTruncated: true,
		HasMergeConflict:           ptrTo(true),
		DetailsLoaded:              true,
	}})[0]
	if out.GetHost() != "ghe.corp" || out.GetAccountLogin() != "bob" {
		t.Fatalf("origin: %+v", out)
	}
	if fc := out.GetFailingChecks(); len(fc) != 1 || fc[0].GetName() != "lint" || fc[0].GetUrl() != "https://l" || fc[0].GetConclusion() != "failure" {
		t.Fatalf("failing checks: %+v", fc)
	}
	if out.HasMergeConflict == nil || !*out.HasMergeConflict || !out.GetDetailsLoaded() || !out.GetUnresolvedThreadsTruncated() || out.GetUnresolvedThreadCount() != 50 {
		t.Fatalf("detail fields: %+v", out)
	}
}

func TestToProtoListUserPRsResponse_should_CarryAccountStatusesOnListAndWatchEvent_When_PollHadFailures(t *testing.T) {
	out := accountStatusesToProto([]gh.AccountPollStatus{
		{Host: "github.com", AccountLogin: "alice", State: gh.AccountPollOK},
		{Host: "ghe.corp", AccountLogin: "bob", State: gh.AccountPollUnauthorized, Detail: "sign-in expired or token rejected"},
		{Host: "x.ghe", AccountLogin: "rita", State: gh.AccountPollRateLimited},
		{Host: "y.ghe", AccountLogin: "eve", State: gh.AccountPollError},
	})
	wantStates := []sessionv1.AccountPollState{
		sessionv1.AccountPollState_ACCOUNT_POLL_STATE_OK,
		sessionv1.AccountPollState_ACCOUNT_POLL_STATE_UNAUTHORIZED,
		sessionv1.AccountPollState_ACCOUNT_POLL_STATE_RATE_LIMITED,
		sessionv1.AccountPollState_ACCOUNT_POLL_STATE_ERROR,
	}
	if len(out) != len(wantStates) {
		t.Fatalf("want %d statuses, got %d", len(wantStates), len(out))
	}
	for i, w := range wantStates {
		if out[i].GetState() != w {
			t.Fatalf("status %d: want %v got %v", i, w, out[i].GetState())
		}
	}
	if out[1].GetHost() != "ghe.corp" || out[1].GetAccountLogin() != "bob" || out[1].GetDetail() == "" {
		t.Fatalf("bob status: %+v", out[1])
	}
	// Both response types carry the same field; an empty cache yields none.
	resp := &sessionv1.ListUserPRsResponse{AccountStatuses: out}
	ev := &sessionv1.UserPREvent{AccountStatuses: out}
	if len(resp.GetAccountStatuses()) != 4 || len(ev.GetAccountStatuses()) != 4 {
		t.Fatal("statuses must be settable on both messages")
	}
}

func ptrTo[T any](v T) *T { return &v }
