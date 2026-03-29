package auth

import "github.com/go-webauthn/webauthn/webauthn"

// singleUser represents the single local user for stapler-squad.
// stapler-squad is a single-user application; all registered passkeys belong
// to the same implicit "owner" account.
const ownerUserID = "stapler-squad-owner"

// localUser implements webauthn.User for the single owner account.
type localUser struct {
	store *CredentialStore
}

func newLocalUser(store *CredentialStore) *localUser {
	return &localUser{store: store}
}

func (u *localUser) WebAuthnID() []byte {
	return []byte(ownerUserID)
}

func (u *localUser) WebAuthnName() string {
	return "owner"
}

func (u *localUser) WebAuthnDisplayName() string {
	return "Stapler Squad Owner"
}

func (u *localUser) WebAuthnCredentials() []webauthn.Credential {
	return u.store.GetCredentials()
}
