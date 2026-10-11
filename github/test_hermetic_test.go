package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHermeticTransport_RefusesNonLoopbackHost(t *testing.T) {
	t.Setenv(allowTestNetworkEnv, "")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := HTTPClient().Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, errTestNetworkDenied) {
		t.Fatalf("Do() err = %v, want errTestNetworkDenied", err)
	}
}

func TestHermeticTransport_AllowsLoopback(t *testing.T) {
	t.Setenv(allowTestNetworkEnv, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := loopbackOnlyTransport{next: http.DefaultTransport}.RoundTrip(req)
	if err != nil {
		t.Fatalf("loopback RoundTrip: %v", err)
	}
	_ = resp.Body.Close()
}

func TestHermeticInit_KeychainStartsEmpty(t *testing.T) {
	// TestMain installs keyring.MockInit too; this pins the contract that a
	// test binary resolves no stored token without any per-test setup.
	if tok := GetKeychainToken(); tok != "" {
		t.Fatal("test binary resolved a keychain token; real keychain is reachable")
	}
}
