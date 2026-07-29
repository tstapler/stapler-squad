package github

import (
	"encoding/json"
	"sync"

	"github.com/zalando/go-keyring"
)

const keychainService = "stapler-squad"
const keychainTokenKey = "github-token"       // legacy single-account key (github.com only)
const keychainAccountsKey = "github-accounts" // JSON list of accounts; see AccountRef
const keychainAccountPrefix = "github-token:" // per-account key prefix

// keychainMu serializes every call into the underlying keyring package.
// go-keyring's own backends (the test-only mockProvider, and the real OS
// backends behind it — macOS Keychain, Secret Service over D-Bus) are not
// guaranteed thread-safe, and this package has two independent, concurrently
// -triggerable callers: RPC handlers (e.g. AddGitHubAccountWithToken) and
// UserPRCache's background refresh loop (loop -> fetch -> resolveAllLogins ->
// collectAllTokens). Guarding every keyring.Get/Set/Delete call here — the
// one place all call sites already funnel through — fixes the race
// regardless of whether a given backend happens to be safe on its own.
//
// A plain Mutex (not RWMutex) is used because keychain access is not a hot
// path: reads happen once per UserPRCache poll interval (default tens of
// seconds) plus occasional RPC calls, so read-read concurrency has no
// measurable benefit here and isn't worth the extra RWMutex complexity.
var keychainMu sync.Mutex

// keyringGet wraps keyring.Get with keychainMu so it never races with a
// concurrent keyringSet/keyringDelete call.
func keyringGet(service, key string) (string, error) {
	keychainMu.Lock()
	defer keychainMu.Unlock()
	return keyring.Get(service, key)
}

// keyringSet wraps keyring.Set with keychainMu so it never races with a
// concurrent keyringGet/keyringDelete call.
func keyringSet(service, key, value string) error {
	keychainMu.Lock()
	defer keychainMu.Unlock()
	return keyring.Set(service, key, value)
}

// keyringDelete wraps keyring.Delete with keychainMu so it never races with a
// concurrent keyringGet/keyringSet call.
func keyringDelete(service, key string) error {
	keychainMu.Lock()
	defer keychainMu.Unlock()
	return keyring.Delete(service, key)
}

// AccountRef identifies one connected GitHub account by username and host.
type AccountRef struct {
	Username string `json:"username"`
	Host     string `json:"host"`
}

// accountKey returns the per-account keychain key for ref. github.com accounts
// keep the legacy key shape ("github-token:<username>") so existing keychain
// entries keep working without migration; enterprise accounts get a
// host-qualified key ("github-token:<host>:<username>").
func accountKey(ref AccountRef) string {
	if IsGitHubCom(ref.Host) {
		return keychainAccountPrefix + ref.Username
	}
	return keychainAccountPrefix + NormalizeHost(ref.Host) + ":" + ref.Username
}

// GetKeychainToken returns any stored GitHub token (first account, or the
// legacy single-account slot). Kept for backward-compatibility with the
// single-token auth flow.
func GetKeychainToken() string {
	for _, ref := range ListKeychainAccounts() {
		if tok := GetKeychainTokenForAccount(ref.Host, ref.Username); tok != "" {
			return tok
		}
	}
	// Fall back to the legacy single-account slot.
	tok, err := keyringGet(keychainService, keychainTokenKey)
	if err != nil {
		return ""
	}
	return tok
}

// SetKeychainToken stores a token under the legacy single-account slot.
// Prefer SetKeychainTokenForAccount when the username is known.
func SetKeychainToken(token string) error {
	return keyringSet(keychainService, keychainTokenKey, token)
}

// DeleteKeychainToken removes the legacy single-account token.
func DeleteKeychainToken() error {
	return keyringDelete(keychainService, keychainTokenKey)
}

// ListKeychainAccounts returns the ordered list of connected GitHub accounts.
// The stored shape is normally []AccountRef; a legacy []string of usernames
// (from before per-host support) is transparently read as github.com accounts.
func ListKeychainAccounts() []AccountRef {
	raw, err := keyringGet(keychainService, keychainAccountsKey)
	if err != nil || raw == "" {
		return nil
	}
	var accounts []AccountRef
	if jsonErr := json.Unmarshal([]byte(raw), &accounts); jsonErr == nil {
		return accounts
	}
	var legacy []string
	if jsonErr := json.Unmarshal([]byte(raw), &legacy); jsonErr != nil {
		return nil
	}
	accounts = make([]AccountRef, len(legacy))
	for i, username := range legacy {
		accounts[i] = AccountRef{Username: username, Host: defaultHost}
	}
	return accounts
}

// GetKeychainTokenForAccount returns the stored token for username on host, or "".
func GetKeychainTokenForAccount(host, username string) string {
	tok, err := keyringGet(keychainService, accountKey(AccountRef{Username: username, Host: host}))
	if err != nil {
		return ""
	}
	return tok
}

// SetKeychainTokenForAccount stores a token under a per-account key and adds
// the account to the accounts list if not already present.
func SetKeychainTokenForAccount(host, username, token string) error {
	ref := AccountRef{Username: username, Host: NormalizeHost(host)}
	if err := keyringSet(keychainService, accountKey(ref), token); err != nil {
		return err
	}
	return addToAccountList(ref)
}

// DeleteKeychainTokenForAccount removes the token for username on host and
// removes it from the accounts list.
func DeleteKeychainTokenForAccount(host, username string) error {
	ref := AccountRef{Username: username, Host: NormalizeHost(host)}
	_ = keyringDelete(keychainService, accountKey(ref))
	return removeFromAccountList(ref)
}

// GetAllKeychainTokens returns all stored tokens across all named accounts plus
// the legacy single-account slot. Each entry is a (username, host, token) tuple.
func GetAllKeychainTokens() []AccountToken {
	seen := make(map[string]bool)
	var out []AccountToken
	for _, ref := range ListKeychainAccounts() {
		if tok := GetKeychainTokenForAccount(ref.Host, ref.Username); tok != "" && !seen[tok] {
			seen[tok] = true
			out = append(out, AccountToken{Username: ref.Username, Host: ref.Host, Token: tok})
		}
	}
	// Legacy slot: include only if not already covered by a named account.
	if tok, err := keyringGet(keychainService, keychainTokenKey); err == nil && tok != "" && !seen[tok] {
		seen[tok] = true
		out = append(out, AccountToken{Username: "", Host: defaultHost, Token: tok})
	}
	return out
}

// AccountToken pairs a GitHub username and host with its token.
type AccountToken struct {
	Username string // empty for the legacy single-account slot
	Host     string
	Token    string
}

// addToAccountList appends ref to the JSON list stored in the keychain
// if it is not already present.
func addToAccountList(ref AccountRef) error {
	accounts := ListKeychainAccounts()
	for _, a := range accounts {
		if a == ref {
			return nil // already in list
		}
	}
	accounts = append(accounts, ref)
	raw, err := json.Marshal(accounts)
	if err != nil {
		return err
	}
	return keyringSet(keychainService, keychainAccountsKey, string(raw))
}

// removeFromAccountList removes ref from the JSON list.
func removeFromAccountList(ref AccountRef) error {
	accounts := ListKeychainAccounts()
	filtered := accounts[:0]
	for _, a := range accounts {
		if a != ref {
			filtered = append(filtered, a)
		}
	}
	if len(filtered) == 0 {
		_ = keyringDelete(keychainService, keychainAccountsKey)
		return nil
	}
	raw, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	return keyringSet(keychainService, keychainAccountsKey, string(raw))
}
