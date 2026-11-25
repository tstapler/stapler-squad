package credentialstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
)

func newCredentials(t testing.TB) *Credentials {
	t.Helper()
	return New(entdbtest.NewClient(t))
}

func TestCredentials_should_AuthenticateOnlyWhileActive_When_IssuedAndRevoked(t *testing.T) {
	t.Parallel()
	c := newCredentials(t)
	ctx := context.Background()
	info := domain.IntegrationCredentialInfo{PrincipalID: "gh-signal", WorkspaceID: "ws-1", AllowedDirRoot: "/tmp/repo"}

	token, err := c.IssueIntegrationCredential(ctx, info)
	require.NoError(t, err)
	assert.Contains(t, token, integrationTokenPrefix)

	got, err := c.AuthenticateIntegrationToken(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, info, *got)

	stored, err := c.client.IntegrationCredential.Query().Only(ctx)
	require.NoError(t, err)
	assert.NotContains(t, stored.TokenSha256, token, "the plaintext token must never be stored")
	assert.Equal(t, hashIntegrationToken(token), stored.TokenSha256)

	_, err = c.AuthenticateIntegrationToken(ctx, token+"x")
	assert.ErrorIs(t, err, domain.ErrNotFound)
	_, err = c.AuthenticateIntegrationToken(ctx, "")
	assert.ErrorIs(t, err, domain.ErrNotFound)

	_, err = c.IssueIntegrationCredential(ctx, info)
	assert.ErrorIs(t, err, domain.ErrConflict, "re-issuing must not silently rotate an existing principal")

	require.NoError(t, c.RevokeIntegrationCredential(ctx, "gh-signal"))
	_, err = c.AuthenticateIntegrationToken(ctx, token)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCredentials_should_RejectIssue_When_AnyRequiredFieldIsEmpty(t *testing.T) {
	t.Parallel()
	c := newCredentials(t)
	for name, info := range map[string]domain.IntegrationCredentialInfo{
		"principal": {WorkspaceID: "ws", AllowedDirRoot: "/r"},
		"workspace": {PrincipalID: "p", AllowedDirRoot: "/r"},
		"root":      {PrincipalID: "p", WorkspaceID: "ws"},
	} {
		_, err := c.IssueIntegrationCredential(context.Background(), info)
		require.Error(t, err, name)
	}
	n, err := c.client.IntegrationCredential.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Zero(t, n, "a rejected issue must not write a row")
}

func TestCredentials_should_ReturnNotFound_When_RevokingUnknownOrAlreadyRevokedPrincipal(t *testing.T) {
	t.Parallel()
	c := newCredentials(t)
	ctx := context.Background()
	assert.ErrorIs(t, c.RevokeIntegrationCredential(ctx, "nobody"), domain.ErrNotFound)

	_, err := c.IssueIntegrationCredential(ctx, domain.IntegrationCredentialInfo{PrincipalID: "p", WorkspaceID: "ws", AllowedDirRoot: "/r"})
	require.NoError(t, err)
	require.NoError(t, c.RevokeIntegrationCredential(ctx, "p"))
	assert.ErrorIs(t, c.RevokeIntegrationCredential(ctx, "p"), domain.ErrNotFound, "the second revoke matches no active row")
}

func TestCredentials_should_KeepOtherPrincipalsActive_When_OneIsRevoked(t *testing.T) {
	t.Parallel()
	c := newCredentials(t)
	ctx := context.Background()
	a, err := c.IssueIntegrationCredential(ctx, domain.IntegrationCredentialInfo{PrincipalID: "a", WorkspaceID: "ws", AllowedDirRoot: "/r"})
	require.NoError(t, err)
	b, err := c.IssueIntegrationCredential(ctx, domain.IntegrationCredentialInfo{PrincipalID: "b", WorkspaceID: "ws", AllowedDirRoot: "/r"})
	require.NoError(t, err)
	require.NotEqual(t, a, b)

	require.NoError(t, c.RevokeIntegrationCredential(ctx, "a"))
	got, err := c.AuthenticateIntegrationToken(ctx, b)
	require.NoError(t, err)
	assert.Equal(t, "b", got.PrincipalID)
}
