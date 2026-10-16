package sshremote

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/ssh"
)

func TestGenerateIdentity_ProducesValidEd25519Keypair(t *testing.T) {
	identity, err := GenerateIdentity("box-a")
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	// PrivateKeyPEM must parse back as a valid SSH signer (ssh.NewSignerFromKey
	// compatible, per Task 3.2.2a).
	signer, err := ssh.ParsePrivateKey(identity.PrivateKeyPEM)
	if err != nil {
		t.Fatalf("ssh.ParsePrivateKey(PrivateKeyPEM) failed: %v", err)
	}
	if signer.PublicKey().Type() != "ssh-ed25519" {
		t.Errorf("signer public key type = %q, want ssh-ed25519", signer.PublicKey().Type())
	}

	// PublicKeyText must match the parsed signer's own public key.
	wantPub := strings.TrimSuffix(string(ssh.MarshalAuthorizedKey(signer.PublicKey())), "\n")
	if identity.PublicKeyText != wantPub {
		t.Errorf("PublicKeyText = %q, want %q", identity.PublicKeyText, wantPub)
	}
	if !strings.HasPrefix(identity.PublicKeyText, "ssh-ed25519 ") {
		t.Errorf("PublicKeyText = %q, want ssh-ed25519 prefix", identity.PublicKeyText)
	}
}

// TestFormatAuthorizedKeysLine_MatchesADR004Format asserts the generated
// line follows ADR-004's recommended shape: a forced command= wrapper plus
// restrict,pty ahead of the bare public key.
func TestFormatAuthorizedKeysLine_MatchesADR004Format(t *testing.T) {
	identity, err := GenerateIdentity("box-a")
	if err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	line := identity.AuthorizedKeysLine
	if !strings.HasPrefix(line, `command="`) {
		t.Errorf("AuthorizedKeysLine = %q, want it to start with a command= forced-command option", line)
	}
	if !strings.Contains(line, ",restrict,") {
		t.Errorf("AuthorizedKeysLine = %q, want it to include restrict", line)
	}
	if !strings.Contains(line, ",pty ") {
		t.Errorf("AuthorizedKeysLine = %q, want it to include pty ahead of the public key", line)
	}
	if !strings.HasSuffix(line, identity.PublicKeyText) {
		t.Errorf("AuthorizedKeysLine = %q, want it to end with PublicKeyText %q", line, identity.PublicKeyText)
	}
}

// TestGenerateIdentity_UniqueAcrossRemotes is the Task 3.2.2c acceptance
// test: two remotes onboarded in sequence must get byte-distinct private
// keys, verified by generating+storing via KeyStore.GenerateAndStoreIdentity
// and reading both back via KeyStore.GetIdentity.
func TestGenerateIdentity_UniqueAcrossRemotes(t *testing.T) {
	keyring.MockInit()
	ks := NewKeyStore()
	ctx := context.Background()

	identityA, err := ks.GenerateAndStoreIdentity(ctx, "box-a")
	if err != nil {
		t.Fatalf("GenerateAndStoreIdentity(box-a) failed: %v", err)
	}
	identityB, err := ks.GenerateAndStoreIdentity(ctx, "box-b")
	if err != nil {
		t.Fatalf("GenerateAndStoreIdentity(box-b) failed: %v", err)
	}

	if bytes.Equal(identityA.PrivateKeyPEM, identityB.PrivateKeyPEM) {
		t.Fatal("GenerateIdentity produced identical private keys for two different remotes")
	}
	if identityA.PublicKeyText == identityB.PublicKeyText {
		t.Fatal("GenerateIdentity produced identical public keys for two different remotes")
	}

	_, valueA, err := ks.GetIdentity(ctx, "box-a")
	if err != nil {
		t.Fatalf("GetIdentity(box-a) failed: %v", err)
	}
	_, valueB, err := ks.GetIdentity(ctx, "box-b")
	if err != nil {
		t.Fatalf("GetIdentity(box-b) failed: %v", err)
	}

	if bytes.Equal(valueA, valueB) {
		t.Fatal("KeyStore.GetIdentity read back identical private keys for two different remotes")
	}
	if !bytes.Equal(valueA, identityA.PrivateKeyPEM) {
		t.Errorf("GetIdentity(box-a) = %q, want %q", valueA, identityA.PrivateKeyPEM)
	}
	if !bytes.Equal(valueB, identityB.PrivateKeyPEM) {
		t.Errorf("GetIdentity(box-b) = %q, want %q", valueB, identityB.PrivateKeyPEM)
	}
}

// TestGenerateOrDescribeIdentity_IsIdempotent is the Add Remote form's
// "clicking Test connection twice must show the same authorized_keys line"
// requirement (ssh-remote-workspaces Phase 6, Epic 6.1): a second call for
// the same name must return byte-identical key material, not silently
// rotate it.
func TestGenerateOrDescribeIdentity_IsIdempotent(t *testing.T) {
	keyring.MockInit()
	ks := NewKeyStore()
	ctx := context.Background()

	first, err := ks.GenerateOrDescribeIdentity(ctx, "prod-box")
	if err != nil {
		t.Fatalf("first GenerateOrDescribeIdentity failed: %v", err)
	}
	second, err := ks.GenerateOrDescribeIdentity(ctx, "prod-box")
	if err != nil {
		t.Fatalf("second GenerateOrDescribeIdentity failed: %v", err)
	}

	if !bytes.Equal(first.PrivateKeyPEM, second.PrivateKeyPEM) {
		t.Error("GenerateOrDescribeIdentity rotated the private key on a repeat call for the same name")
	}
	if first.PublicKeyText != second.PublicKeyText {
		t.Errorf("PublicKeyText changed across repeat calls: %q != %q", first.PublicKeyText, second.PublicKeyText)
	}
	if first.AuthorizedKeysLine != second.AuthorizedKeysLine {
		t.Errorf("AuthorizedKeysLine changed across repeat calls: %q != %q", first.AuthorizedKeysLine, second.AuthorizedKeysLine)
	}

	// A different name must still get its own distinct identity.
	other, err := ks.GenerateOrDescribeIdentity(ctx, "gpu-box")
	if err != nil {
		t.Fatalf("GenerateOrDescribeIdentity(gpu-box) failed: %v", err)
	}
	if other.PublicKeyText == first.PublicKeyText {
		t.Error("GenerateOrDescribeIdentity returned the same key for two different remote names")
	}
}

// TestGenerateOrDescribeIdentity_ConcurrentCallsForSameNewName_ReturnIdenticalIdentity
// is the regression test for the TOCTOU race a fix-first review caught: the
// GetIdentity check and the GenerateAndStoreIdentity write weren't atomic as
// a pair (only each individual keyring call was serialized via the
// package-level keyringMu), so two concurrent callers for a brand-new
// remote name -- e.g. a rapid double-click on "Test connection" in the Add
// Remote form -- could both miss the check before either had written
// anything, each generate a DIFFERENT keypair, and race on which write
// landed last. generateOrDescribeMu (see GenerateOrDescribeIdentity's doc
// comment) now serializes the whole check-then-generate-then-store sequence,
// so every concurrent caller for the same name must observe the identical
// identity -- never a silently-overwritten second keypair.
func TestGenerateOrDescribeIdentity_ConcurrentCallsForSameNewName_ReturnIdenticalIdentity(t *testing.T) {
	keyring.MockInit()
	ks := NewKeyStore()
	ctx := context.Background()

	const n = 20
	results := make([]GeneratedIdentity, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = ks.GenerateOrDescribeIdentity(ctx, "race-box")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}

	first := results[0]
	for i := 1; i < n; i++ {
		if results[i].PublicKeyText != first.PublicKeyText {
			t.Errorf("call %d returned a different public key than call 0: %q != %q", i, results[i].PublicKeyText, first.PublicKeyText)
		}
		if !bytes.Equal(results[i].PrivateKeyPEM, first.PrivateKeyPEM) {
			t.Errorf("call %d returned a different private key than call 0 -- concurrent GenerateOrDescribeIdentity calls raced", i)
		}
	}

	// The stored identity must also match what every caller observed --
	// confirms the LAST write didn't silently diverge from what was
	// returned to callers who read before it landed.
	_, stored, err := ks.GetIdentity(ctx, "race-box")
	if err != nil {
		t.Fatalf("GetIdentity(race-box) failed: %v", err)
	}
	if !bytes.Equal(stored, first.PrivateKeyPEM) {
		t.Error("the stored identity diverged from the identity returned to concurrent callers")
	}
}
