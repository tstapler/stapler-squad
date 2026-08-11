package github

import (
	"context"
	"testing"

	"github.com/zalando/go-keyring"
)

// resetGHTokenCache clears getGHToken's package-level 1-minute token cache
// (ghTokenCacheVal/ghTokenCacheAt in http_client.go) so each subtest's
// keyring.MockInit() call actually takes effect. Without this, a subtest that
// populates the cache via getGHToken's keychain fallback leaks its cached
// value into a later subtest within the same minute, even after that later
// subtest re-initializes the mock keyring with different (or no) data.
func resetGHTokenCache() {
	ghTokenCacheVal.Store("")
	ghTokenCacheAt.Store(0)
}

// TestGetGHTokenForAccount covers all 4 resolution branches of
// getGHTokenForAccount: {github.com, non-github.com} x {username set, username
// empty}. TestMain (github/main_test.go) already clears GITHUB_TOKEN/GH_TOKEN
// and switches go-keyring to its in-memory mock for the whole package, so each
// subtest only needs to seed the keychain state it cares about via
// SetKeychainTokenForAccount / SetKeychainToken — plus reset the getGHToken
// cache (see resetGHTokenCache) since that cache is package-global and
// otherwise survives across subtests within the same process.
func TestGetGHTokenForAccount(t *testing.T) {
	ctx := context.Background()

	t.Run("github.com with username returns the per-account keychain token", func(t *testing.T) {
		keyring.MockInit() // fresh in-memory store per subtest
		resetGHTokenCache()
		if err := SetKeychainTokenForAccount("github.com", "alice", "alice-token"); err != nil {
			t.Fatalf("SetKeychainTokenForAccount failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: "github.com", Username: "alice"})
		if got != "alice-token" {
			t.Errorf("getGHTokenForAccount() = %q, want %q", got, "alice-token")
		}
	})

	t.Run("github.com with username falls back to getGHToken when no per-account token exists", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()
		// No per-account token for "bob"; seed the legacy single-account slot,
		// which GetKeychainToken() (called via getGHToken's fallback chain)
		// reads as a last resort.
		if err := SetKeychainToken("legacy-token"); err != nil {
			t.Fatalf("SetKeychainToken failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: "github.com", Username: "bob"})
		if got != "legacy-token" {
			t.Errorf("getGHTokenForAccount() = %q, want %q", got, "legacy-token")
		}
	})

	t.Run("github.com with empty username uses getGHToken directly", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()
		if err := SetKeychainToken("default-token"); err != nil {
			t.Fatalf("SetKeychainToken failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: "github.com", Username: ""})
		if got != "default-token" {
			t.Errorf("getGHTokenForAccount() = %q, want %q", got, "default-token")
		}
	})

	t.Run("empty host behaves like github.com (defaults through getGHToken)", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()
		if err := SetKeychainToken("default-token"); err != nil {
			t.Fatalf("SetKeychainToken failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: "", Username: ""})
		if got != "default-token" {
			t.Errorf("getGHTokenForAccount() = %q, want %q", got, "default-token")
		}
	})

	t.Run("non-github.com host with username returns the per-account keychain token", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()
		const host = "github.example.com"
		if err := SetKeychainTokenForAccount(host, "carol", "carol-enterprise-token"); err != nil {
			t.Fatalf("SetKeychainTokenForAccount failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: host, Username: "carol"})
		if got != "carol-enterprise-token" {
			t.Errorf("getGHTokenForAccount() = %q, want %q", got, "carol-enterprise-token")
		}
	})

	t.Run("non-github.com host with username and no stored token returns empty (no getGHToken fallback)", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()
		// Even though a github.com/legacy token exists, an enterprise host with
		// a named but untokened account must NOT fall back to it.
		if err := SetKeychainToken("should-not-be-returned"); err != nil {
			t.Fatalf("SetKeychainToken failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: "github.example.com", Username: "dave"})
		if got != "" {
			t.Errorf("getGHTokenForAccount() = %q, want empty string", got)
		}
	})

	t.Run("non-github.com host with empty username returns any token stored for that host", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()
		const host = "github.example.com"
		if err := SetKeychainTokenForAccount(host, "erin", "erin-host-token"); err != nil {
			t.Fatalf("SetKeychainTokenForAccount failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: host, Username: ""})
		if got != "erin-host-token" {
			t.Errorf("getGHTokenForAccount() = %q, want %q", got, "erin-host-token")
		}
	})

	t.Run("non-github.com host with empty username and no stored token returns empty", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()

		got := getGHTokenForAccount(ctx, AccountRef{Host: "github.example.com", Username: ""})
		if got != "" {
			t.Errorf("getGHTokenForAccount() = %q, want empty string", got)
		}
	})
}
