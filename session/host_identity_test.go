package session

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ssqlog "github.com/tstapler/stapler-squad/log"
)

// captureLogInfo temporarily redirects the log package's injectable slog seam to a
// text handler writing into the returned buffer, restoring the previous handler
// via t.Cleanup. Mirrors main_test.go's captureLogWarn for this package.
func captureLogInfo(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		ssqlog.SetSlogDefaultForTest(prev)
	})
	return &buf
}

func TestLoadOrCreateHostIdentity_should_MintAndPersist_When_NoIdentityFileExists(t *testing.T) {
	stateDir := t.TempDir()

	identity, err := LoadOrCreateHostIdentity(stateDir)
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}
	if !identity.ID.IsValid() {
		t.Fatalf("LoadOrCreateHostIdentity() minted an invalid HostID")
	}
	if len(identity.PublicKey) == 0 || len(identity.PrivateKey) == 0 {
		t.Fatalf("LoadOrCreateHostIdentity() minted an identity with an empty keypair")
	}

	path := filepath.Join(stateDir, hostIdentityFileName)
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("expected %s to be persisted, stat error = %v", path, statErr)
	}
}

func TestLoadOrCreateHostIdentity_should_ReturnClearError_When_IdentityFileCorrupted(t *testing.T) {
	stateDir := t.TempDir()
	path := filepath.Join(stateDir, hostIdentityFileName)
	if err := os.WriteFile(path, []byte("{not valid json"), 0600); err != nil {
		t.Fatalf("failed to seed corrupted identity file: %v", err)
	}

	_, err := LoadOrCreateHostIdentity(stateDir)
	if err == nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = nil, want a corruption error")
	}
}

func TestLoadOrCreateHostIdentity_should_ReturnSameIdentity_When_LoadedTwiceAcrossRestarts(t *testing.T) {
	stateDir := t.TempDir()

	first, err := LoadOrCreateHostIdentity(stateDir)
	if err != nil {
		t.Fatalf("first LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	second, err := LoadOrCreateHostIdentity(stateDir)
	if err != nil {
		t.Fatalf("second LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	if first.ID.String() != second.ID.String() {
		t.Fatalf("HostID changed across simulated restarts: first = %s, second = %s", first.ID, second.ID)
	}
	if string(first.PublicKey) != string(second.PublicKey) {
		t.Fatalf("PublicKey changed across simulated restarts")
	}
	if string(first.PrivateKey) != string(second.PrivateKey) {
		t.Fatalf("PrivateKey changed across simulated restarts")
	}
}

func TestLoadOrCreateHostIdentity_should_LogHostIdentityGenerated_When_MintingNewIdentity(t *testing.T) {
	stateDir := t.TempDir()
	buf := captureLogInfo(t)

	identity, err := LoadOrCreateHostIdentity(stateDir)
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	got := buf.String()
	if !strings.Contains(got, "host_identity.generated") {
		t.Fatalf("expected a host_identity.generated log line, got: %s", got)
	}
	if !strings.Contains(got, identity.ID.String()) {
		t.Fatalf("expected the log line to name the minted host id %s, got: %s", identity.ID.String(), got)
	}
}

func TestLoadOrCreateHostIdentity_should_NotLogHostIdentityGenerated_When_LoadingExistingIdentity(t *testing.T) {
	stateDir := t.TempDir()

	if _, err := LoadOrCreateHostIdentity(stateDir); err != nil {
		t.Fatalf("initial LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	buf := captureLogInfo(t)
	if _, err := LoadOrCreateHostIdentity(stateDir); err != nil {
		t.Fatalf("second LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	if strings.Contains(buf.String(), "host_identity.generated") {
		t.Fatalf("expected no host_identity.generated log line on the load-existing path, got: %s", buf.String())
	}
}

func TestHostIdentity_Sign_should_ProduceVerifiableSignature_When_UsingOwnKeypair(t *testing.T) {
	stateDir := t.TempDir()
	identity, err := LoadOrCreateHostIdentity(stateDir)
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	payload := []byte("host advertisement payload")
	sig := identity.Sign(payload)

	if !VerifyAdvertisement(identity.PublicKey, payload, sig) {
		t.Fatalf("VerifyAdvertisement() = false, want true for a signature produced by the matching private key")
	}
}

func TestVerifyAdvertisement_should_RejectSignature_When_PayloadTampered(t *testing.T) {
	stateDir := t.TempDir()
	identity, err := LoadOrCreateHostIdentity(stateDir)
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	sig := identity.Sign([]byte("original payload"))

	if VerifyAdvertisement(identity.PublicKey, []byte("tampered payload"), sig) {
		t.Fatalf("VerifyAdvertisement() = true, want false when the payload has been tampered with after signing")
	}
}

func TestVerifyAdvertisement_should_RejectSignature_When_KeyMismatch(t *testing.T) {
	stateDirA := t.TempDir()
	stateDirB := t.TempDir()

	identityA, err := LoadOrCreateHostIdentity(stateDirA)
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity(A) error = %v, want nil", err)
	}
	identityB, err := LoadOrCreateHostIdentity(stateDirB)
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity(B) error = %v, want nil", err)
	}

	payload := []byte("host advertisement payload")
	sig := identityA.Sign(payload)

	if VerifyAdvertisement(identityB.PublicKey, payload, sig) {
		t.Fatalf("VerifyAdvertisement() = true, want false when verifying against a different identity's public key")
	}
}
