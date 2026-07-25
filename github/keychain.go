package github

import (
	"encoding/json"

	"github.com/zalando/go-keyring"
)

const keychainService = "stapler-squad"
const keychainTokenKey = "github-token"       // legacy single-account key
const keychainAccountsKey = "github-accounts" // JSON []string of usernames
const keychainAccountPrefix = "github-token:" // per-account key prefix

// GetKeychainToken returns any stored GitHub token (first account, or the
// legacy single-account slot). Kept for backward-compatibility with the
// single-token auth flow.
func GetKeychainToken() string {
	accounts := ListKeychainAccounts()
	for _, username := range accounts {
		if tok := GetKeychainTokenForAccount(username); tok != "" {
			return tok
		}
	}
	// Fall back to the legacy single-account slot.
	tok, err := keyring.Get(keychainService, keychainTokenKey)
	if err != nil {
		return ""
	}
	return tok
}

// SetKeychainToken stores a token under the legacy single-account slot.
// Prefer SetKeychainTokenForAccount when the username is known.
func SetKeychainToken(token string) error {
	return keyring.Set(keychainService, keychainTokenKey, token)
}

// DeleteKeychainToken removes the legacy single-account token.
func DeleteKeychainToken() error {
	return keyring.Delete(keychainService, keychainTokenKey)
}

// ListKeychainAccounts returns the ordered list of connected GitHub usernames.
func ListKeychainAccounts() []string {
	raw, err := keyring.Get(keychainService, keychainAccountsKey)
	if err != nil || raw == "" {
		return nil
	}
	var accounts []string
	if jsonErr := json.Unmarshal([]byte(raw), &accounts); jsonErr != nil {
		return nil
	}
	return accounts
}

// GetKeychainTokenForAccount returns the stored token for the given username, or "".
func GetKeychainTokenForAccount(username string) string {
	tok, err := keyring.Get(keychainService, keychainAccountPrefix+username)
	if err != nil {
		return ""
	}
	return tok
}

// SetKeychainTokenForAccount stores a token under a per-username key and adds
// the username to the accounts list if not already present.
func SetKeychainTokenForAccount(username, token string) error {
	if err := keyring.Set(keychainService, keychainAccountPrefix+username, token); err != nil {
		return err
	}
	return addToAccountList(username)
}

// DeleteKeychainTokenForAccount removes the token for username and removes it
// from the accounts list.
func DeleteKeychainTokenForAccount(username string) error {
	_ = keyring.Delete(keychainService, keychainAccountPrefix+username)
	return removeFromAccountList(username)
}

// GetAllKeychainTokens returns all stored tokens across all named accounts plus
// the legacy single-account slot. Each entry is a (username, token) pair.
func GetAllKeychainTokens() []AccountToken {
	seen := make(map[string]bool)
	var out []AccountToken
	for _, username := range ListKeychainAccounts() {
		if tok := GetKeychainTokenForAccount(username); tok != "" && !seen[tok] {
			seen[tok] = true
			out = append(out, AccountToken{Username: username, Token: tok})
		}
	}
	// Legacy slot: include only if not already covered by a named account.
	if tok, err := keyring.Get(keychainService, keychainTokenKey); err == nil && tok != "" && !seen[tok] {
		seen[tok] = true
		out = append(out, AccountToken{Username: "", Token: tok})
	}
	return out
}

// AccountToken pairs a GitHub username with its token.
type AccountToken struct {
	Username string // empty for the legacy single-account slot
	Token    string
}

// addToAccountList appends username to the JSON list stored in the keychain
// if it is not already present.
func addToAccountList(username string) error {
	accounts := ListKeychainAccounts()
	for _, a := range accounts {
		if a == username {
			return nil // already in list
		}
	}
	accounts = append(accounts, username)
	raw, err := json.Marshal(accounts)
	if err != nil {
		return err
	}
	return keyring.Set(keychainService, keychainAccountsKey, string(raw))
}

// removeFromAccountList removes username from the JSON list.
func removeFromAccountList(username string) error {
	accounts := ListKeychainAccounts()
	filtered := accounts[:0]
	for _, a := range accounts {
		if a != username {
			filtered = append(filtered, a)
		}
	}
	if len(filtered) == 0 {
		_ = keyring.Delete(keychainService, keychainAccountsKey)
		return nil
	}
	raw, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	return keyring.Set(keychainService, keychainAccountsKey, string(raw))
}
