package services

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/jules"
)

// stubJulesTokens is a test double for the julesKeyringTokenSource interface
// JulesCredentialSource depends on, so these tests never touch a real OS
// keychain.
type stubJulesTokens struct {
	key jules.JulesAPIKey
	err error
}

func (s stubJulesTokens) APIKey(context.Context) (jules.JulesAPIKey, error) {
	return s.key, s.err
}

func TestCredentialChain_Resolve_should_ReturnKeychainCredential_When_ProviderIsJules(t *testing.T) {
	chain := NewChain(&JulesCredentialSource{tokens: stubJulesTokens{key: jules.JulesAPIKey("AIzaSyD-EXAMPLE")}})

	cred, err := chain.Resolve(context.Background(), "jules")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cred.Source != "jules_keychain" {
		t.Errorf("Source = %q, want %q", cred.Source, "jules_keychain")
	}
	if cred.APIKey != "AIzaSyD-EXAMPLE" {
		t.Errorf("APIKey = %q, want %q", cred.APIKey, "AIzaSyD-EXAMPLE")
	}
}

func TestCredentialChain_Resolve_should_PreferEnvVarOverKeychain_When_JulesAPIKeyEnvSet(t *testing.T) {
	t.Setenv("JULES_API_KEY", "env-key")

	chain := NewChain(
		&EnvVarCredentialSource{},
		&JulesCredentialSource{tokens: stubJulesTokens{key: jules.JulesAPIKey("keychain-key")}},
	)

	cred, err := chain.Resolve(context.Background(), "jules")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cred.APIKey != "env-key" {
		t.Errorf("APIKey = %q, want %q (env var must win over keychain)", cred.APIKey, "env-key")
	}
}
