package sshremote

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// wrapperScriptPlaceholder stands in for the path to a server-side scoping
// wrapper script in the generated authorized_keys line. Per ADR-004
// (authorized-keys-scoping-is-recommendation-not-enforcement), Stapler
// Squad generates this line as a recommendation for the user to install and
// adapt themselves -- it cannot install, verify, or enforce a real wrapper
// script on the remote host, so no attempt is made to resolve this to an
// actual path.
const wrapperScriptPlaceholder = "/path/to/stapler-squad-ssh-wrapper.sh"

// GeneratedIdentity is the result of generating a fresh per-remote Ed25519
// SSH keypair.
type GeneratedIdentity struct {
	// PrivateKeyPEM is the OpenSSH-format PEM-encoded private key, suitable
	// for both KeyStore.SetIdentity and ssh.ParsePrivateKey/
	// ssh.NewSignerFromKey-compatible consumption once parsed back.
	PrivateKeyPEM []byte
	// PublicKeyText is the bare authorized_keys-format public key line
	// ("ssh-ed25519 AAAA...", no trailing newline, no options).
	PublicKeyText string
	// AuthorizedKeysLine is PublicKeyText prefixed with the ADR-004
	// recommended command=/restrict/pty scoping options, ready to display
	// to the user during remote onboarding (Phase 6, Epic 6.1) as
	// copy-paste text -- not something this package installs anywhere.
	AuthorizedKeysLine string
}

// GenerateIdentity generates a fresh Ed25519 keypair for remoteName. Each
// call produces a byte-distinct keypair (crypto/rand-backed), so generating
// per-remote rather than reusing one key everywhere limits a single
// compromised key's blast radius to one remote (research/pitfalls.md §3).
func GenerateIdentity(remoteName string) (GeneratedIdentity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return GeneratedIdentity{}, fmt.Errorf("sshremote: generate ed25519 keypair for %q: %w", remoteName, err)
	}

	block, err := ssh.MarshalPrivateKey(priv, remoteName)
	if err != nil {
		return GeneratedIdentity{}, fmt.Errorf("sshremote: marshal private key for %q: %w", remoteName, err)
	}
	privateKeyPEM := pem.EncodeToMemory(block)

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return GeneratedIdentity{}, fmt.Errorf("sshremote: derive public key for %q: %w", remoteName, err)
	}
	publicKeyText := strings.TrimSuffix(string(ssh.MarshalAuthorizedKey(sshPub)), "\n")

	return GeneratedIdentity{
		PrivateKeyPEM:      privateKeyPEM,
		PublicKeyText:      publicKeyText,
		AuthorizedKeysLine: formatAuthorizedKeysLine(publicKeyText),
	}, nil
}

// formatAuthorizedKeysLine formats the ADR-004 recommended authorized_keys
// line: a forced command= wrapper plus restrict (disables agent/port/X11
// forwarding by default, OpenSSH 7.2+) and pty (re-enabled for interactive
// tmux) ahead of the bare public key text.
func formatAuthorizedKeysLine(publicKeyText string) string {
	return fmt.Sprintf(`command=%q,restrict,pty %s`, wrapperScriptPlaceholder, publicKeyText)
}

// GenerateAndStoreIdentity generates a fresh Ed25519 keypair for remoteName,
// stores the private key via KeyStore.SetIdentity, and returns the full
// GeneratedIdentity (including the public key text and recommended
// authorized_keys line) for the onboarding flow to display.
func (ks *KeyStore) GenerateAndStoreIdentity(ctx context.Context, remoteName string) (GeneratedIdentity, error) {
	identity, err := GenerateIdentity(remoteName)
	if err != nil {
		return GeneratedIdentity{}, err
	}
	if err := ks.SetIdentity(ctx, remoteName, IdentityKindPrivateKey, identity.PrivateKeyPEM); err != nil {
		return GeneratedIdentity{}, fmt.Errorf("sshremote: store generated identity for %q: %w", remoteName, err)
	}
	return identity, nil
}
