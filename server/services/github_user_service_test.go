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
	gh.EnterpriseBaseURLOverride[enterpriseHost] = ts.URL + "/"
	defer delete(gh.EnterpriseBaseURLOverride, enterpriseHost)

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
