// Package jira stores JIRA Cloud credentials (email + API token) per base URL
// in the OS keychain so they never land in PluginConfig.Raw JSON.
package jira

import (
	"encoding/json"
	"net/url"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

const keychainService = "stapler-squad"
const keychainKeyPrefix = "jira-token:"

// keychainMu serializes all keyring access (see github/keychain.go).
var keychainMu sync.Mutex

type credential struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

// NormalizeHost reduces a base URL ("https://Acme.atlassian.net/") to its
// lower-cased host ("acme.atlassian.net"), so equivalent spellings share a key.
func NormalizeHost(baseURL string) string {
	s := strings.TrimSpace(baseURL)
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		return strings.ToLower(u.Host)
	}
	return strings.ToLower(strings.TrimSuffix(s, "/"))
}

func keyFor(baseURL string) string { return keychainKeyPrefix + NormalizeHost(baseURL) }

// GetKeychainTokenForHost returns the stored (email, token) for baseURL;
// ok is false when none is stored.
func GetKeychainTokenForHost(baseURL string) (email, token string, ok bool) {
	keychainMu.Lock()
	defer keychainMu.Unlock()
	raw, err := keyring.Get(keychainService, keyFor(baseURL))
	if err != nil {
		return "", "", false
	}
	var c credential
	if json.Unmarshal([]byte(raw), &c) != nil || c.Token == "" {
		return "", "", false
	}
	return c.Email, c.Token, true
}

// SetKeychainTokenForHost stores the (email, token) pair for baseURL as one
// JSON keyring value.
func SetKeychainTokenForHost(baseURL, email, token string) error {
	raw, err := json.Marshal(credential{Email: email, Token: token})
	if err != nil {
		return err
	}
	keychainMu.Lock()
	defer keychainMu.Unlock()
	return keyring.Set(keychainService, keyFor(baseURL), string(raw))
}

// DeleteKeychainTokenForHost removes the credentials stored for baseURL.
func DeleteKeychainTokenForHost(baseURL string) error {
	keychainMu.Lock()
	defer keychainMu.Unlock()
	return keyring.Delete(keychainService, keyFor(baseURL))
}
