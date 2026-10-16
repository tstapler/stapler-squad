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

// keychainMu guards every call into the keyring package. go-keyring's
// mockProvider (github.com/zalando/go-keyring@v0.2.8/keyring_mock.go) backs
// ALL keys with a single unsynchronized mockStore map[string]map[string]string
// — Get/Set/Delete for two different keys still read/write that same shared
// map with no internal locking, so a per-key lock does not prevent a race
// there; only serializing all access does. Verified directly: switching to a
// per-(service,key) lock registry made `go test -race ./github/...` fail
// TestSetKeychainTokenForAccount_NoRace_When_ConcurrentWithListAndGetAllTokens
// and TestDeleteKeychainTokenForAccount_NoRace_When_ConcurrentWithReads with a
// genuine data race inside mockProvider (2026-08-14). Live mutex profiling
// separately showed this lock's ~37% of total mutex delay sits almost
// entirely inside keyring.Get's OS Keychain round-trip rather than lock-
// acquisition overhead, so narrowing scope wouldn't reduce delay anyway — the
// slow work itself must run serialized because the dependency it calls into
// isn't safe to call concurrently across keys.
var keychainMu sync.Mutex

func keyringGet(service, key string) (string, error) {
	keychainMu.Lock()
	defer keychainMu.Unlock()
	return keyring.Get(service, key)
}

func keyringSet(service, key, value string) error {
	keychainMu.Lock()
	defer keychainMu.Unlock()
	return keyring.Set(service, key, value)
}

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

// GetKeychainToken returns a token for the default single-token auth flow
// (getGHToken/newGHRequest), which always targets api.github.com. It must
// therefore prefer a github.com account's token over any other configured
// host: returning an enterprise account's token here sends valid credentials
// to the wrong API and GitHub correctly rejects them as 401 Bad credentials,
// regardless of account order in ListKeychainAccounts. Falls back to the
// first account of any host, then the legacy single-account slot, only when
// no github.com account is configured.
func GetKeychainToken() string {
	if tok := GetKeychainTokenForHost(defaultHost); tok != "" {
		return tok
	}
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

// GetKeychainTokenForHost returns any stored token for host, regardless of
// which account it belongs to. Backlog sync plugins (session/backlog_plugin_github.go,
// session/backlog_plugin_github_prs.go) only know a host, not a username, so they
// can't call GetKeychainTokenForAccount directly. Falls back to the legacy
// single-account slot for github.com when no named account matches.
func GetKeychainTokenForHost(host string) string {
	normalized := NormalizeHost(host)
	for _, ref := range ListKeychainAccounts() {
		if NormalizeHost(ref.Host) == normalized {
			if tok := GetKeychainTokenForAccount(ref.Host, ref.Username); tok != "" {
				return tok
			}
		}
	}
	if IsGitHubCom(host) {
		if tok, err := keyringGet(keychainService, keychainTokenKey); err == nil {
			return tok
		}
	}
	return ""
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
