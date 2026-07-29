package github

import (
	"strconv"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestSetKeychainTokenForAccount_NoRace_When_ConcurrentWithListAndGetAllTokens
// is the BUG-052 regression test. It reproduces the exact race the bug
// documented: SetKeychainTokenForAccount (write path, e.g. an
// AddGitHubAccountWithToken RPC call) running concurrently with
// ListKeychainAccounts / GetAllKeychainTokens (read path, e.g. UserPRCache's
// background refresh loop -> fetch -> resolveAllLogins -> collectAllTokens).
//
// Before the fix (keychainMu guarding every keyring.Get/Set/Delete call in
// this file), `go test -race` fails this test with a DATA RACE between
// mockProvider.Set and mockProvider.Get. After the fix, it passes clean.
func TestSetKeychainTokenForAccount_NoRace_When_ConcurrentWithListAndGetAllTokens(t *testing.T) {
	keyring.MockInit()

	const writers = 20
	const readers = 20

	var wg sync.WaitGroup
	wg.Add(writers + readers)

	for i := 0; i < writers; i++ {
		i := i
		go func() {
			defer wg.Done()
			username := "writer-" + strconv.Itoa(i)
			if err := SetKeychainTokenForAccount("github.com", username, "token-"+strconv.Itoa(i)); err != nil {
				t.Errorf("SetKeychainTokenForAccount(%q) failed: %v", username, err)
			}
		}()
	}

	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			// Exercise both read paths implicated in the original race trace:
			// ListKeychainAccounts (github/keychain.go:63 in the bug trace) and
			// GetAllKeychainTokens (github/keychain.go:114 in the bug trace, via
			// collectAllTokens in user_pr_cache.go).
			_ = ListKeychainAccounts()
			_ = GetAllKeychainTokens()
		}()
	}

	wg.Wait()

	// Sanity: every writer's token actually landed, proving the mutex serializes
	// access without silently dropping writes.
	for i := 0; i < writers; i++ {
		username := "writer-" + strconv.Itoa(i)
		got := GetKeychainTokenForAccount("github.com", username)
		want := "token-" + strconv.Itoa(i)
		if got != want {
			t.Errorf("GetKeychainTokenForAccount(%q) = %q, want %q", username, got, want)
		}
	}
}

// TestDeleteKeychainTokenForAccount_NoRace_When_ConcurrentWithReads covers the
// delete path (DeleteKeychainTokenForAccount / removeFromAccountList), which
// the original bug trace did not exercise directly but uses the same
// unguarded keyring.Delete call sites.
func TestDeleteKeychainTokenForAccount_NoRace_When_ConcurrentWithReads(t *testing.T) {
	keyring.MockInit()

	const accounts = 20
	for i := 0; i < accounts; i++ {
		if err := SetKeychainTokenForAccount("github.com", "acct-"+strconv.Itoa(i), "tok"); err != nil {
			t.Fatalf("setup SetKeychainTokenForAccount failed: %v", err)
		}
	}

	var wg sync.WaitGroup
	wg.Add(accounts * 2)
	for i := 0; i < accounts; i++ {
		i := i
		go func() {
			defer wg.Done()
			if err := DeleteKeychainTokenForAccount("github.com", "acct-"+strconv.Itoa(i)); err != nil {
				t.Errorf("DeleteKeychainTokenForAccount failed: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			_ = ListKeychainAccounts()
			_ = GetAllKeychainTokens()
		}()
	}
	wg.Wait()
}
