package sshremote

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// newTestHostKey generates a throwaway Ed25519 SSH public key for tests.
func newTestHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("derive ssh public key: %v", err)
	}
	return sshPub
}

// newTestKnownHostsStore builds a KnownHostsStore backed by a fresh temp
// file under t.TempDir(), isolated from every other test (and from the real
// ~/.stapler-squad/ssh_known_hosts) regardless of config.GetConfigDir()'s
// own test-mode isolation.
func newTestKnownHostsStore(t *testing.T) *KnownHostsStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ssh_known_hosts")
	s, err := NewKnownHostsStore(withKnownHostsPath(path))
	if err != nil {
		t.Fatalf("NewKnownHostsStore: %v", err)
	}
	return s
}

func TestNewKnownHostsStore_UsesConfigGetConfigDir_When_NoPathOverride(t *testing.T) {
	// No withKnownHostsPath override: exercises the real
	// config.GetConfigDir()-based path resolution (isolated per-test-process
	// automatically since this runs under `go test`, per
	// config.IsTestMode()).
	s, err := NewKnownHostsStore()
	if err != nil {
		t.Fatalf("NewKnownHostsStore: %v", err)
	}
	if !strings.HasSuffix(s.path, knownHostsFileName) {
		t.Fatalf("expected path to end with %q, got %q", knownHostsFileName, s.path)
	}
	if _, err := os.Stat(s.path); err != nil {
		t.Fatalf("expected known_hosts file to be created, stat failed: %v", err)
	}
}

func TestVerify_ReturnsErrUnknownHostKey_When_HostNeverSeen(t *testing.T) {
	store := newTestKnownHostsStore(t)
	key := newTestHostKey(t)

	err := store.Verify("prod.example.com", key)
	if err == nil {
		t.Fatal("expected an error for a never-seen host, got nil")
	}

	var unknownErr *ErrUnknownHostKey
	if !errors.As(err, &unknownErr) {
		t.Fatalf("expected *ErrUnknownHostKey, got %T: %v", err, err)
	}
	if unknownErr.Host != "prod.example.com" {
		t.Errorf("Host = %q, want %q", unknownErr.Host, "prod.example.com")
	}
	wantFingerprint := HostKeyFingerprint(key)
	if unknownErr.Fingerprint != wantFingerprint {
		t.Errorf("Fingerprint = %q, want %q", unknownErr.Fingerprint, wantFingerprint)
	}
	if !strings.HasPrefix(unknownErr.Fingerprint, "SHA256:") {
		t.Errorf("Fingerprint = %q, want SHA256:... display format", unknownErr.Fingerprint)
	}

	var mismatchErr *ErrHostKeyMismatch
	if errors.As(err, &mismatchErr) {
		t.Fatalf("unknown host must not also match ErrHostKeyMismatch, got %v", mismatchErr)
	}
}

func TestVerify_ReturnsNil_When_HostPreviouslyTrusted(t *testing.T) {
	store := newTestKnownHostsStore(t)
	key := newTestHostKey(t)

	if err := store.Trust("prod.example.com", key); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	if err := store.Verify("prod.example.com", key); err != nil {
		t.Fatalf("Verify after Trust: expected nil, got %v", err)
	}
}

func TestVerify_ReturnsErrHostKeyMismatch_NotErrUnknownHostKey_When_KeyChangedSinceTrust(t *testing.T) {
	store := newTestKnownHostsStore(t)
	original := newTestHostKey(t)
	rotated := newTestHostKey(t)

	if err := store.Trust("prod.example.com", original); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	err := store.Verify("prod.example.com", rotated)
	if err == nil {
		t.Fatal("expected an error for a changed host key, got nil")
	}

	var mismatchErr *ErrHostKeyMismatch
	if !errors.As(err, &mismatchErr) {
		t.Fatalf("expected *ErrHostKeyMismatch (the MITM-relevant case), got %T: %v", err, err)
	}
	wantFingerprint := HostKeyFingerprint(rotated)
	if mismatchErr.Fingerprint != wantFingerprint {
		t.Errorf("Fingerprint = %q, want fingerprint of the NEW key %q", mismatchErr.Fingerprint, wantFingerprint)
	}

	// Security-critical: a key mismatch must never be reported as merely
	// "unknown" -- that would let a caller route it into the same "safe to
	// trust and connect" UI flow as a legitimate first-time connection.
	var unknownErr *ErrUnknownHostKey
	if errors.As(err, &unknownErr) {
		t.Fatalf("key mismatch must not be reported as ErrUnknownHostKey, got %v", unknownErr)
	}
}

func TestTrust_ReplacesStaleEntry_So_OldKeyNoLongerVerifies(t *testing.T) {
	store := newTestKnownHostsStore(t)
	original := newTestHostKey(t)
	rotated := newTestHostKey(t)

	if err := store.Trust("prod.example.com", original); err != nil {
		t.Fatalf("Trust(original): %v", err)
	}
	if err := store.Trust("prod.example.com", rotated); err != nil {
		t.Fatalf("Trust(rotated): %v", err)
	}

	// The new key must verify cleanly.
	if err := store.Verify("prod.example.com", rotated); err != nil {
		t.Fatalf("Verify(rotated) after re-Trust: expected nil, got %v", err)
	}

	// The OLD key must no longer verify -- Trust replaces, it doesn't
	// accumulate, so a rollback to the previously-valid (now revoked) key
	// is reported as a mismatch, not silently accepted.
	err := store.Verify("prod.example.com", original)
	var mismatchErr *ErrHostKeyMismatch
	if !errors.As(err, &mismatchErr) {
		t.Fatalf("Verify(original) after re-Trust: expected *ErrHostKeyMismatch, got %v", err)
	}
}

func TestVerify_And_Trust_HandleHostWithoutExplicitPort(t *testing.T) {
	// The acceptance criteria call Verify with a bare hostname (no port);
	// this must not error out on knownhosts' internal SplitHostPort
	// requirements.
	store := newTestKnownHostsStore(t)
	key := newTestHostKey(t)

	err := store.Verify("prod.example.com", key)
	var unknownErr *ErrUnknownHostKey
	if !errors.As(err, &unknownErr) {
		t.Fatalf("expected *ErrUnknownHostKey, got %T: %v", err, err)
	}

	if err := store.Trust("prod.example.com", key); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	if err := store.Verify("prod.example.com", key); err != nil {
		t.Fatalf("Verify after Trust: expected nil, got %v", err)
	}
	// A "host:port" form (matching what a real SSH dial's HostKeyCallback
	// receives, per session/tmux.SSHTarget.Addr) must resolve to the same
	// trust decision as the bare form.
	if err := store.Verify("prod.example.com:22", key); err != nil {
		t.Fatalf("Verify(\"host:22\") after Trust(\"host\"): expected nil, got %v", err)
	}
}

func TestHostKeyCallback_ReturnsRawKnownHostsError_NotPreTranslated(t *testing.T) {
	// HostKeyCallback is documented to hand back the RAW *knownhosts.KeyError
	// (not ErrUnknownHostKey) so tmux.NewSSHRunner's own wrapHostKeyCallback
	// remains the single translation point for the real-dial path.
	store := newTestKnownHostsStore(t)
	key := newTestHostKey(t)
	cb := store.HostKeyCallback()

	err := cb("prod.example.com:22", fakeAddr("prod.example.com:22"), key)
	if err == nil {
		t.Fatal("expected an error for a never-seen host, got nil")
	}

	var unknownErr *ErrUnknownHostKey
	if errors.As(err, &unknownErr) {
		t.Fatalf("HostKeyCallback must return the raw knownhosts error, not a pre-translated ErrUnknownHostKey; got %v", unknownErr)
	}

	var keyErr interface{ Error() string }
	if !errors.As(err, &keyErr) {
		t.Fatalf("expected an error, got %T", err)
	}
}

func TestHostKeyCallback_ReturnsNil_When_HostPreviouslyTrusted(t *testing.T) {
	store := newTestKnownHostsStore(t)
	key := newTestHostKey(t)

	if err := store.Trust("prod.example.com", key); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	cb := store.HostKeyCallback()
	if err := cb("prod.example.com:22", fakeAddr("prod.example.com:22"), key); err != nil {
		t.Fatalf("HostKeyCallback after Trust: expected nil, got %v", err)
	}
}

// TestIsHostKeyMismatch_ClassifiesBothCallPathsIdentically pins the exact
// bug a prior remote_service.go regression hit: HostKeyCallback() (the
// tmux.SSHRunner dial path) and Verify() (the direct-call path) surface a
// mismatch as two DIFFERENT concrete error shapes -- the raw
// *knownhosts.KeyError versus *ErrHostKeyMismatch -- so a caller checking
// errors.As against only one of them (as server/services/remote_service.go
// used to) silently misses the other. IsHostKeyMismatch must return true
// for both, and false for the unrelated "unknown host" and "nil" cases.
func TestIsHostKeyMismatch_ClassifiesBothCallPathsIdentically(t *testing.T) {
	store := newTestKnownHostsStore(t)
	original := newTestHostKey(t)
	rotated := newTestHostKey(t)
	if err := store.Trust("prod.example.com", original); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	// Path 1: HostKeyCallback() -- what TestRemoteConnection's SSH dial
	// actually receives on a mismatch (the RAW *knownhosts.KeyError).
	cbErr := store.HostKeyCallback()("prod.example.com:22", fakeAddr("prod.example.com:22"), rotated)
	if cbErr == nil {
		t.Fatal("expected an error from HostKeyCallback for a rotated key")
	}
	if !IsHostKeyMismatch(cbErr) {
		t.Errorf("IsHostKeyMismatch(cbErr) = false, want true for the raw knownhosts.KeyError HostKeyCallback returns")
	}

	// Path 2: Verify() -- wraps the same underlying error in
	// *ErrHostKeyMismatch.
	verifyErr := store.Verify("prod.example.com", rotated)
	if verifyErr == nil {
		t.Fatal("expected an error from Verify for a rotated key")
	}
	if !IsHostKeyMismatch(verifyErr) {
		t.Errorf("IsHostKeyMismatch(verifyErr) = false, want true for *ErrHostKeyMismatch")
	}

	// Negative cases: must not fire for "unknown host" or nil.
	unknownErr := store.Verify("never-seen.example.com", rotated)
	if IsHostKeyMismatch(unknownErr) {
		t.Errorf("IsHostKeyMismatch(unknownErr) = true, want false for an unknown-host error")
	}
	if IsHostKeyMismatch(nil) {
		t.Error("IsHostKeyMismatch(nil) = true, want false")
	}
}

// TestIsHostTrusted_ReturnsFalse_When_HostNeverSeen covers the case
// server/services.RemoteService.CreateRemote's regression test
// (TestCreateRemote_UntrustedHost_ReturnsFailedPrecondition) exercises
// indirectly through the RPC layer -- this pins the store-level behavior
// directly.
func TestIsHostTrusted_ReturnsFalse_When_HostNeverSeen(t *testing.T) {
	store := newTestKnownHostsStore(t)

	trusted, err := store.IsHostTrusted("never-seen.example.com")
	if err != nil {
		t.Fatalf("IsHostTrusted: %v", err)
	}
	if trusted {
		t.Error("IsHostTrusted() = true, want false for a host with no Trust() call ever made")
	}
}

// TestIsHostTrusted_ReturnsTrue_When_HostTrusted proves IsHostTrusted
// recognizes an existing trust entry WITHOUT being given the actual trusted
// key (its whole point -- see its doc comment for why CreateRemote needs
// exactly this, key-agnostic, question).
func TestIsHostTrusted_ReturnsTrue_When_HostTrusted(t *testing.T) {
	store := newTestKnownHostsStore(t)
	key := newTestHostKey(t)

	if err := store.Trust("prod.example.com", key); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	trusted, err := store.IsHostTrusted("prod.example.com")
	if err != nil {
		t.Fatalf("IsHostTrusted: %v", err)
	}
	if !trusted {
		t.Error("IsHostTrusted() = false, want true for a host Trust() was already called for")
	}
}

// TestIsHostTrusted_IsIndependentOfWhichKeyIsTrusted proves the "any key"
// framing precisely: even a host whose CURRENTLY trusted key differs from
// whatever the caller might eventually present (e.g. a rotated/mismatched
// key -- TestVerify_ReturnsErrHostKeyMismatch_NotErrUnknownHostKey_When_
// KeyChangedSinceTrust's scenario) still reports trusted=true here, since
// IsHostTrusted only asks "has Trust ever run for this host," not "does
// this specific key match."
func TestIsHostTrusted_IsIndependentOfWhichKeyIsTrusted(t *testing.T) {
	store := newTestKnownHostsStore(t)

	if err := store.Trust("prod.example.com", newTestHostKey(t)); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	// A DIFFERENT key than the one trusted -- would fail Verify, but
	// IsHostTrusted doesn't take a key at all.
	rotated := newTestHostKey(t)
	if err := store.Verify("prod.example.com", rotated); err == nil {
		t.Fatal("sanity check failed: Verify should reject the rotated key")
	}

	trusted, err := store.IsHostTrusted("prod.example.com")
	if err != nil {
		t.Fatalf("IsHostTrusted: %v", err)
	}
	if !trusted {
		t.Error("IsHostTrusted() = false, want true -- an entry exists for this host regardless of which key it pins")
	}
}
