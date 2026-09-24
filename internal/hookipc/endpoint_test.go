package hookipc

import (
	"errors"
	"os"
	"testing"
)

func TestResolveEndpoint_should_ReturnDistinctHookEndpoints_When_ConfigDirectoriesDiffer(t *testing.T) {
	clearEndpointEnvironment(t)
	firstDir := t.TempDir()
	secondDir := t.TempDir()

	t.Setenv("STAPLER_SQUAD_TEST_DIR", firstDir)
	first, err := ResolveEndpoint("/repo")
	if err != nil {
		t.Fatalf("ResolveEndpoint(first): %v", err)
	}
	if err := os.Setenv("STAPLER_SQUAD_TEST_DIR", secondDir); err != nil {
		t.Fatal(err)
	}
	second, err := ResolveEndpoint("/repo")
	if err != nil {
		t.Fatalf("ResolveEndpoint(second): %v", err)
	}
	if first.SocketPath == second.SocketPath {
		t.Fatalf("socket paths match: %q", first.SocketPath)
	}
	if first.InstanceFingerprint == second.InstanceFingerprint {
		t.Fatal("instance fingerprints match for distinct state directories")
	}
	if len(first.SocketPath) >= 100 || len(second.SocketPath) >= 100 {
		t.Fatalf("socket paths exceed conservative Unix limit: %d, %d", len(first.SocketPath), len(second.SocketPath))
	}
}

func TestResolveEndpoint_should_NotReturnDefaultEndpoint_When_IsolatedEndpointIsMissing(t *testing.T) {
	clearEndpointEnvironment(t)
	isolatedDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", isolatedDir)

	endpoint, err := ResolveEndpoint("/repo")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.InstanceFingerprint != Fingerprint(isolatedDir) {
		t.Fatalf("fingerprint = %q, want isolated directory fingerprint", endpoint.InstanceFingerprint)
	}
	if _, err := os.Stat(endpoint.SocketPath); !os.IsNotExist(err) {
		t.Fatalf("ResolveEndpoint should not require or substitute a listener; stat error = %v", err)
	}
}

func TestResolveEndpoint_should_RejectInjectedIdentity_When_ItDoesNotMatchResolvedState(t *testing.T) {
	clearEndpointEnvironment(t)
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	t.Setenv(FingerprintEnvironment, string(Fingerprint("/production")))
	t.Setenv(SocketEnvironment, "/tmp/production.sock")

	_, err := ResolveEndpoint("/repo")
	if !errors.Is(err, ErrInstanceMismatch) {
		t.Fatalf("ResolveEndpoint error = %v, want ErrInstanceMismatch", err)
	}
}

func clearEndpointEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv(SocketEnvironment, "")
	t.Setenv(FingerprintEnvironment, "")
	t.Setenv(ProtocolEnvironment, "")
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
}
