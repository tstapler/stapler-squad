package github

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mustPRKey(t *testing.T, host, owner, repo string, n int) PRKey {
	t.Helper()
	k, err := NewPRKey(host, owner, repo, n)
	require.NoError(t, err)
	return k
}

func seedCache(c *UserPRCache, prs []UserPR, accounts []connectedAccount) {
	c.snapshot.Store(&userPRSnapshot{prs: prs, capturedAt: time.Now()})
	c.multiLogin.Store(&multiLoginState{accounts: accounts, checkedAt: time.Now()})
}

func TestTokenForPR_should_ReturnOwningAccountToken_When_TwoAccountsTwoHostsAndSameHostCarol(t *testing.T) {
	c := newTestCache()
	seedCache(c,
		[]UserPR{
			{Owner: "acme", Repo: "api", Number: 42, Host: "github.com", AccountLogin: "carol"},
			{Owner: "corp", Repo: "svc", Number: 7, Host: "ghe.corp", AccountLogin: "bob"},
			{Owner: "alice", Repo: "own", Number: 1, Host: "github.com", AccountLogin: "alice"},
		},
		[]connectedAccount{
			{token: "tok-alice", login: "alice", host: "github.com"},
			{token: "tok-carol", login: "carol", host: "github.com"},
			{token: "tok-bob", login: "bob", host: "ghe.corp"},
		})

	tok, login, ok := c.TokenForPR(mustPRKey(t, "ghe.corp", "corp", "svc", 7))
	require.True(t, ok)
	require.Equal(t, "tok-bob", tok)
	require.Equal(t, "bob", login)

	// Same host as alice's default token, but carol's poll produced the PR.
	tok, login, ok = c.TokenForPR(mustPRKey(t, "", "ACME", "API", 42))
	require.True(t, ok)
	require.Equal(t, "tok-carol", tok)
	require.Equal(t, "carol", login)
}

func TestTokenForPR_should_ReturnNotOk_When_PRUnknown(t *testing.T) {
	c := newTestCache()
	key := mustPRKey(t, "github.com", "acme", "api", 42)

	_, _, ok := c.TokenForPR(key)
	require.False(t, ok, "empty cache")

	seedCache(c,
		[]UserPR{{Owner: "acme", Repo: "api", Number: 42, Host: "github.com", AccountLogin: "carol"}},
		[]connectedAccount{{token: "tok-carol", login: "carol", host: "github.com"}})
	for name, k := range map[string]PRKey{
		"other number": mustPRKey(t, "github.com", "acme", "api", 43),
		"other host":   mustPRKey(t, "ghe.corp", "acme", "api", 42),
		"other repo":   mustPRKey(t, "github.com", "acme", "web", 42),
		"zero key":     {},
	} {
		_, _, ok := c.TokenForPR(k)
		require.False(t, ok, name)
	}
}

func TestTokenForPR_should_ReturnFirstAccountInOrderAndNeverFallBack_When_PRVisibleToTwoAccounts(t *testing.T) {
	c := newTestCache()
	accounts := []connectedAccount{
		{token: "tok-alice", login: "alice", host: "github.com"},
		{token: "tok-carol", login: "carol", host: "github.com"},
	}
	c.multiLogin.Store(&multiLoginState{accounts: accounts, checkedAt: time.Now()})
	// Both tokens see the same org PR.
	startGitHubCom(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(prResponse(`{"number":42,"title":"t","url":"https://x/acme/api/pull/42","headRefName":"h","baseRefName":"main","state":"OPEN","isDraft":false,"updatedAt":"2026-01-01T00:00:00Z","closedAt":"","mergedAt":"","repository":{"owner":{"login":"acme"},"name":"api"},"reviewDecision":"","reviews":{"nodes":[]}}`)))
	}))
	key := mustPRKey(t, "github.com", "acme", "api", 42)

	for poll := 1; poll <= 3; poll++ {
		require.NoError(t, c.fetch())
		prs := c.GetAll()
		require.Len(t, prs, 1)
		require.Equal(t, "alice", prs[0].AccountLogin, "poll %d", poll)
		tok, login, ok := c.TokenForPR(key)
		require.True(t, ok)
		require.Equal(t, "tok-alice", tok, "poll %d", poll)
		require.Equal(t, "alice", login)
	}

	// Owner no longer connected: no fallback to carol's token.
	c.multiLogin.Store(&multiLoginState{accounts: accounts[1:], checkedAt: time.Now()})
	_, _, ok := c.TokenForPR(key)
	require.False(t, ok)
}
