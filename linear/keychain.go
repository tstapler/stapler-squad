// Package linear stores the Linear API key in the OS keychain so it never
// lands in PluginConfig.Raw JSON.
package linear

import (
	"sync"

	"github.com/zalando/go-keyring"
)

const keychainService = "stapler-squad"
const keychainTokenKey = "linear-api-key"

// keychainMu serializes all keyring access: go-keyring's mock provider backs
// every key with one unsynchronized map (see github/keychain.go for the
// verified race this guards against).
var keychainMu sync.Mutex

// GetKeychainToken returns the stored Linear API key, or "" if none is set.
func GetKeychainToken() string {
	keychainMu.Lock()
	defer keychainMu.Unlock()
	tok, err := keyring.Get(keychainService, keychainTokenKey)
	if err != nil {
		return ""
	}
	return tok
}

// SetKeychainToken stores the Linear API key.
func SetKeychainToken(token string) error {
	keychainMu.Lock()
	defer keychainMu.Unlock()
	return keyring.Set(keychainService, keychainTokenKey, token)
}

// DeleteKeychainToken removes the stored Linear API key.
func DeleteKeychainToken() error {
	keychainMu.Lock()
	defer keychainMu.Unlock()
	return keyring.Delete(keychainService, keychainTokenKey)
}
