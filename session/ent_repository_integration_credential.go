package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/integrationcredential"
)

// integrationTokenPrefix marks a token as an integration credential so it is
// recognisable in leaked-secret scans and never confused with a passkey session token.
const integrationTokenPrefix = "sqi_"

// integrationTokenBytes is the CSPRNG entropy per token. At 256 bits an unsalted SHA-256
// of the token is not brute-forceable, which is why only the hash is stored.
const integrationTokenBytes = 32

// IntegrationCredentialInfo is what a successfully authenticated token resolves to.
type IntegrationCredentialInfo struct {
	PrincipalID    string
	WorkspaceID    string
	AllowedDirRoot string
}

func hashIntegrationToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// IssueIntegrationCredential creates a credential described by info and returns the
// plaintext token. The token is returned exactly once and is not recoverable afterwards.
// PrincipalID is unique: re-issuing for an existing principal fails rather than silently
// rotating, so a leaked token can only be replaced via an explicit revoke.
func (r *EntRepository) IssueIntegrationCredential(ctx context.Context, info IntegrationCredentialInfo) (string, error) {
	if info.PrincipalID == "" || info.WorkspaceID == "" || info.AllowedDirRoot == "" {
		return "", fmt.Errorf("principal, workspace and allowed directory root are required")
	}
	raw := make([]byte, integrationTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate integration token: %w", err)
	}
	token := integrationTokenPrefix + base64.RawURLEncoding.EncodeToString(raw)

	_, err := r.client.IntegrationCredential.Create().
		SetPrincipalID(info.PrincipalID).
		SetTokenSha256(hashIntegrationToken(token)).
		SetWorkspaceID(info.WorkspaceID).
		SetAllowedDirRoot(info.AllowedDirRoot).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return "", fmt.Errorf("%w: credential for principal %q already exists", ErrConflict, info.PrincipalID)
		}
		return "", fmt.Errorf("create integration credential: %w", err)
	}
	return token, nil
}

// AuthenticateIntegrationToken resolves a bearer token to its credential. An unknown or
// revoked token returns ErrNotFound, deliberately indistinguishable from each other.
func (r *EntRepository) AuthenticateIntegrationToken(ctx context.Context, token string) (*IntegrationCredentialInfo, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	cred, err := r.client.IntegrationCredential.Query().
		Where(
			integrationcredential.TokenSha256(hashIntegrationToken(token)),
			integrationcredential.RevokedAtIsNil(),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lookup integration credential: %w", err)
	}
	return &IntegrationCredentialInfo{
		PrincipalID:    cred.PrincipalID,
		WorkspaceID:    cred.WorkspaceID,
		AllowedDirRoot: cred.AllowedDirRoot,
	}, nil
}

// RevokeIntegrationCredential disables the principal's credential. Its registrations are
// left in place; revoking authority is separate from tearing down what it created.
func (r *EntRepository) RevokeIntegrationCredential(ctx context.Context, principalID string) error {
	n, err := r.client.IntegrationCredential.Update().
		Where(integrationcredential.PrincipalID(principalID), integrationcredential.RevokedAtIsNil()).
		SetRevokedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("revoke integration credential: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
