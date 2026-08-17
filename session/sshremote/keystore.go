// Package sshremote stores per-remote SSH identity material (private keys
// and passphrases) in the OS keychain, and generates fresh per-remote
// keypairs at onboarding. See project_plans/ssh-remote-workspaces/implementation/plan.md
// Epic 3.2 for the design this package implements.
package sshremote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
)

// keychainService is this package's OS-keychain service namespace. It is
// deliberately distinct from github/keychain.go's "stapler-squad" service --
// a different package, a different credential domain, no shared entries.
const keychainService = "stapler-squad-ssh"

// keyPrefix namespaces every key this package writes under keychainService,
// mirroring github/keychain.go's keychainAccountPrefix ("github-token:")
// shape but under this package's own service so the two can never collide
// even if they were ever pointed at the same keychainService by mistake.
const keyPrefix = "ssh-key:"

// defaultIdentityTimeout bounds how long GetIdentity/SetIdentity/DeleteIdentity
// wait for the OS keyring before giving up, per pre-mortem.md Failure #3:
// on a headless system, go-keyring's Linux Secret Service backend can hang
// indefinitely on a D-Bus unlock prompt that never appears. Credential
// reads/writes are not long-running operations, so 5s is generous while
// still bounding CreateSession's worst case. Mirrors
// session/tmux/ssh_runner.go's DialTimeout-class budgeting.
const defaultIdentityTimeout = 5 * time.Second

// keyringMu guards every call into the keyring package. Mirrors
// github/keychain.go's keychainMu exactly (see that file's doc comment for
// why serialization must be global rather than per-key: go-keyring's mock
// backend's mockStore is an unsynchronized shared map, so only serializing
// ALL access -- not per-key locking -- prevents a race across different
// keys). This is this package's own, unshared sync.Mutex: a distinct
// contention domain from github's keychainMu, per Task 3.2.1b.
var keyringMu sync.Mutex

// keyringGetFunc, keyringSetFunc, and keyringDeleteFunc are test seams over
// the real github.com/zalando/go-keyring package-level functions. Production
// code never reassigns these -- they default straight through to the real
// keyring. Tests override them to simulate two failure modes that can't be
// reproduced with go-keyring's own MockInit()/MockInitWithError() (which
// only ever fail or succeed immediately):
//
//  1. A keyring call that never returns -- the Linux Secret Service backend
//     blocking on a D-Bus unlock prompt with no session to answer it
//     (pre-mortem.md Failure #3). See TestGetIdentity_ReturnsDeadlineError_When_KeyringHangs.
//  2. "No backend available" reported immediately, to assert the fail-loud
//     path (no silent on-disk fallback exists -- see raceKeyringOp/the
//     KeyStore methods below, which have no fallback code path at all).
var (
	keyringGetFunc    = keyring.Get
	keyringSetFunc    = keyring.Set
	keyringDeleteFunc = keyring.Delete
)

func keyringGet(service, key string) (string, error) {
	keyringMu.Lock()
	defer keyringMu.Unlock()
	return keyringGetFunc(service, key)
}

func keyringSet(service, key, value string) error {
	keyringMu.Lock()
	defer keyringMu.Unlock()
	return keyringSetFunc(service, key, value)
}

func keyringDelete(service, key string) error {
	keyringMu.Lock()
	defer keyringMu.Unlock()
	return keyringDeleteFunc(service, key)
}

// ErrIdentityNotFound is returned by GetIdentity/DeleteIdentity when no
// identity is stored for the given remote name. It is go-keyring's
// ErrNotFound re-exported under this package so callers can errors.Is
// against a name that doesn't leak the underlying keyring library.
var ErrIdentityNotFound = keyring.ErrNotFound

// IdentityKind tags which of the two logical value kinds an identity
// envelope holds -- a raw private key, or a passphrase protecting one.
// Both kinds live under the same key namespace (keyPrefix + remote name);
// the tag is what lets GetIdentity tell them apart on read-back.
type IdentityKind string

const (
	// IdentityKindPrivateKey tags an envelope holding raw private key bytes
	// (e.g. an OpenSSH-format PEM block).
	IdentityKindPrivateKey IdentityKind = "private_key"
	// IdentityKindPassphrase tags an envelope holding a passphrase that
	// protects an encrypted private key.
	IdentityKindPassphrase IdentityKind = "passphrase"
)

// identityEnvelope is the small tagged JSON envelope stored under each
// keychain entry, per Task 3.2.1c. encoding/json marshals a []byte field as
// a standard-base64 string automatically, so Value round-trips arbitrary
// binary key material without any manual encoding step here.
type identityEnvelope struct {
	Kind  IdentityKind `json:"kind"`
	Value []byte       `json:"value"`
}

// identityKey returns the keychain key for remoteName.
func identityKey(remoteName string) string {
	return keyPrefix + remoteName
}

// KeyStore stores SSH identity material (private keys and passphrases) in
// the OS keychain, keyed per remote name -- never in a file under
// ~/.stapler-squad/, and never with an on-disk fallback if the keychain is
// unavailable (per research/build-vs-buy.md §3: fail loud instead).
type KeyStore struct {
	timeout time.Duration
}

// KeyStoreOption configures a KeyStore at construction time.
type KeyStoreOption func(*KeyStore)

// withIdentityTimeout overrides the ctx-race timeout budget applied to every
// KeyStore method call. Unexported: only same-package tests need a
// faster-than-production budget to keep the hang-simulation test
// (TestGetIdentity_ReturnsDeadlineError_When_KeyringHangs) fast.
func withIdentityTimeout(d time.Duration) KeyStoreOption {
	return func(ks *KeyStore) { ks.timeout = d }
}

// NewKeyStore constructs a KeyStore.
func NewKeyStore(opts ...KeyStoreOption) *KeyStore {
	ks := &KeyStore{timeout: defaultIdentityTimeout}
	for _, opt := range opts {
		opt(ks)
	}
	return ks
}

// raceKeyringOp runs op in a goroutine and races it against a ctx bounded to
// ks.timeout, returning a wrapped deadline error if op hasn't finished in
// time. Mirrors session/tmux/ssh_runner.go's newSession/Run ctx-race
// technique exactly, including its documented gap: op cannot be force-
// aborted mid-flight (go-keyring exposes no cancellation), so on timeout the
// background goroutine keeps running until the real (or simulated) keyring
// call returns on its own. Because it still holds keyringMu for that entire
// window, every other KeyStore call blocks on keyringMu until it finishes --
// same trade-off Run/newSession accept for the shared *ssh.Client, applied
// here to the shared keyring backend for the same reason: partially
// serialized access to a non-cancellable dependency beats reintroducing a
// second, competing in-process store.
func (ks *KeyStore) raceKeyringOp(ctx context.Context, op func() error) error {
	ctx, cancel := context.WithTimeout(ctx, ks.timeout)
	defer cancel()

	ch := make(chan error, 1)
	go func() { ch <- op() }()

	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		return fmt.Errorf("sshremote: keychain operation timed out after %s: %w", ks.timeout, ctx.Err())
	}
}

// SetIdentity stores value (either raw private key bytes or a passphrase,
// per kind) for remoteName in the OS keychain, tagged with kind so
// GetIdentity can distinguish the two on read-back.
func (ks *KeyStore) SetIdentity(ctx context.Context, remoteName string, kind IdentityKind, value []byte) error {
	if remoteName == "" {
		return errors.New("sshremote: SetIdentity: remoteName must not be empty")
	}
	if kind != IdentityKindPrivateKey && kind != IdentityKindPassphrase {
		return fmt.Errorf("sshremote: SetIdentity: unknown identity kind %q", kind)
	}

	raw, err := json.Marshal(identityEnvelope{Kind: kind, Value: value})
	if err != nil {
		return fmt.Errorf("sshremote: encode identity envelope for %q: %w", remoteName, err)
	}

	if err := ks.raceKeyringOp(ctx, func() error {
		return keyringSet(keychainService, identityKey(remoteName), string(raw))
	}); err != nil {
		return fmt.Errorf("sshremote: set identity for %q: %w", remoteName, err)
	}
	return nil
}

// GetIdentity returns the stored identity kind and value for remoteName.
// Returns an error wrapping ErrIdentityNotFound if nothing is stored.
func (ks *KeyStore) GetIdentity(ctx context.Context, remoteName string) (IdentityKind, []byte, error) {
	var raw string
	if err := ks.raceKeyringOp(ctx, func() error {
		var getErr error
		raw, getErr = keyringGet(keychainService, identityKey(remoteName))
		return getErr
	}); err != nil {
		return "", nil, fmt.Errorf("sshremote: get identity for %q: %w", remoteName, err)
	}

	var envelope identityEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return "", nil, fmt.Errorf("sshremote: decode identity envelope for %q: %w", remoteName, err)
	}
	return envelope.Kind, envelope.Value, nil
}

// DeleteIdentity removes the stored identity for remoteName. Returns an
// error wrapping ErrIdentityNotFound if nothing was stored.
func (ks *KeyStore) DeleteIdentity(ctx context.Context, remoteName string) error {
	if err := ks.raceKeyringOp(ctx, func() error {
		return keyringDelete(keychainService, identityKey(remoteName))
	}); err != nil {
		return fmt.Errorf("sshremote: delete identity for %q: %w", remoteName, err)
	}
	return nil
}
