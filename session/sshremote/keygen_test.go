package sshremote

import (
	"bytes"
	"context"
	"strings"
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
