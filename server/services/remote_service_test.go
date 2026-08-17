package services

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"testing"

	"connectrpc.com/connect"
	gliderssh "github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/sshremote"
)

// startRemoteTestSSHServer starts a minimal in-process SSH server on an
// OS-assigned loopback port, accepting any client public key (this test
// suite is exercising Epic 3.3's host-key TOFU flow, not client
// authentication). A package-local helper rather than an import from
// session/tmux or session/git: those packages' own SSH test servers
// (ssh_test_server_test.go) live in _test.go files, which Go doesn't
// compile into an importable package across package boundaries.
func startRemoteTestSSHServer(t *testing.T) (addr string, hostKey ssh.PublicKey) {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromSigner(priv)
	require.NoError(t, err)

	srv := &gliderssh.Server{
		Handler: func(s gliderssh.Session) { _ = s.Exit(0) },
		PublicKeyHandler: func(gliderssh.Context, gliderssh.PublicKey) bool {
			return true
		},
	}
	srv.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})

	return ln.Addr().String(), signer.PublicKey()
}

// newRemoteTestKey generates a throwaway Ed25519 SSH public key, e.g. to
// stand in for a host key that was trusted under a now-stale identity.
func newRemoteTestKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sshPub, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return sshPub
}

// newTestRemoteService builds a RemoteService backed by a real
// KnownHostsStore/KeyStore (both test-isolated: KnownHostsStore via
// config.GetConfigDir()'s test-mode auto-detection, KeyStore via go-keyring's
// mock backend) and a fixed config.Config.
func newTestRemoteService(t *testing.T, cfg *config.Config) (*RemoteService, *sshremote.KnownHostsStore, *sshremote.KeyStore) {
	t.Helper()
	keyring.MockInit()

	knownHosts, err := sshremote.NewKnownHostsStore()
	require.NoError(t, err)
	keyStore := sshremote.NewKeyStore()

	svc := NewRemoteService(knownHosts, keyStore, func() *config.Config { return cfg })
	return svc, knownHosts, keyStore
}

func TestTestRemoteConnection_ReturnsHostKeyUnknown_When_NeverSeen(t *testing.T) {
	addr, hostKey := startRemoteTestSSHServer(t)
	cfg := &config.Config{Remotes: []config.RemoteConfig{
		{Name: "test-remote", Host: addr, User: "testuser"},
	}}
	svc, _, _ := newTestRemoteService(t, cfg)

	resp, err := svc.TestRemoteConnection(context.Background(), connect.NewRequest(&sessionv1.TestRemoteConnectionRequest{
		RemoteName: "test-remote",
	}))
	require.NoError(t, err)

	require.False(t, resp.Msg.Success)
	require.True(t, resp.Msg.HostKeyUnknown)
	require.Equal(t, sshremote.HostKeyFingerprint(hostKey), resp.Msg.Fingerprint)
	require.Empty(t, resp.Msg.ErrorMessage)
}

func TestTrustRemoteHostKey_Then_TestRemoteConnection_RoundTripSucceeds(t *testing.T) {
	addr, hostKey := startRemoteTestSSHServer(t)
	cfg := &config.Config{Remotes: []config.RemoteConfig{
		{Name: "test-remote", Host: addr, User: "testuser", IdentityRef: "test-remote"},
	}}
	svc, knownHosts, keyStore := newTestRemoteService(t, cfg)

	// Register an identity so the post-trust round trip can fully
	// authenticate (the test server's PublicKeyHandler accepts any key).
	_, err := keyStore.GenerateAndStoreIdentity(context.Background(), "test-remote")
	require.NoError(t, err)

	// 1. First attempt: host key unknown.
	first, err := svc.TestRemoteConnection(context.Background(), connect.NewRequest(&sessionv1.TestRemoteConnectionRequest{
		RemoteName: "test-remote",
	}))
	require.NoError(t, err)
	require.True(t, first.Msg.HostKeyUnknown)
	fingerprint := first.Msg.Fingerprint
	require.NotEmpty(t, fingerprint)

	// 2. Trust it.
	trustResp, err := svc.TrustRemoteHostKey(context.Background(), connect.NewRequest(&sessionv1.TrustRemoteHostKeyRequest{
		RemoteName:  "test-remote",
		Fingerprint: fingerprint,
	}))
	require.NoError(t, err)
	require.True(t, trustResp.Msg.Success, "TrustRemoteHostKey error: %s", trustResp.Msg.ErrorMessage)

	// Directly confirm the store now verifies the real host key -- not just
	// that the RPC claimed success.
	require.NoError(t, knownHosts.Verify(addr, hostKey))

	// 3. Second attempt: fully succeeds (host key trusted + auth via the
	// stored identity).
	second, err := svc.TestRemoteConnection(context.Background(), connect.NewRequest(&sessionv1.TestRemoteConnectionRequest{
		RemoteName: "test-remote",
	}))
	require.NoError(t, err)
	require.False(t, second.Msg.HostKeyUnknown)
	require.Empty(t, second.Msg.ErrorMessage)
	require.True(t, second.Msg.Success)
}

func TestTrustRemoteHostKey_RejectsMismatchedFingerprint(t *testing.T) {
	addr, _ := startRemoteTestSSHServer(t)
	cfg := &config.Config{Remotes: []config.RemoteConfig{
		{Name: "test-remote", Host: addr, User: "testuser"},
	}}
	svc, knownHosts, _ := newTestRemoteService(t, cfg)

	resp, err := svc.TrustRemoteHostKey(context.Background(), connect.NewRequest(&sessionv1.TrustRemoteHostKeyRequest{
		RemoteName:  "test-remote",
		Fingerprint: "SHA256:not-the-real-fingerprint-at-all",
	}))
	require.NoError(t, err)
	require.False(t, resp.Msg.Success)
	require.NotEmpty(t, resp.Msg.ErrorMessage)

	// The mismatched fingerprint must not have been trusted -- the host is
	// still reported unknown.
	unknownErr := knownHosts.Verify(addr, newRemoteTestKey(t))
	require.Error(t, unknownErr)
	var unknownHostErr *sshremote.ErrUnknownHostKey
	require.ErrorAs(t, unknownErr, &unknownHostErr)
}

func TestTestRemoteConnection_ReportsMismatch_NotHostKeyUnknown_When_TrustedKeyChanged(t *testing.T) {
	addr, realHostKey := startRemoteTestSSHServer(t)
	cfg := &config.Config{Remotes: []config.RemoteConfig{
		{Name: "test-remote", Host: addr, User: "testuser"},
	}}
	svc, knownHosts, _ := newTestRemoteService(t, cfg)

	// Trust a DIFFERENT key than the one the real test server presents --
	// simulating a remote whose host key rotated (or a MITM) since it was
	// last trusted.
	staleKey := newRemoteTestKey(t)
	require.NotEqual(t, sshremote.HostKeyFingerprint(staleKey), sshremote.HostKeyFingerprint(realHostKey))
	require.NoError(t, knownHosts.Trust(addr, staleKey))

	resp, err := svc.TestRemoteConnection(context.Background(), connect.NewRequest(&sessionv1.TestRemoteConnectionRequest{
		RemoteName: "test-remote",
	}))
	require.NoError(t, err)

	// Security-critical: a key mismatch must be reported as a failure, and
	// specifically NOT as host_key_unknown -- conflating the two would let
	// the frontend offer the same "trust and connect" flow for a possible
	// MITM as for a legitimate first connection.
	require.False(t, resp.Msg.Success)
	require.False(t, resp.Msg.HostKeyUnknown)
	// Assert on the actual message CONTENT, not just non-emptiness: a prior
	// version of hostKeyAwareErrorMessage checked for the wrong error type
	// on this call path (errors.As against *sshremote.ErrHostKeyMismatch,
	// which TestRemoteConnection's dial -- via HostKeyCallback() -- never
	// produces; see sshremote.IsHostKeyMismatch's doc comment) and silently
	// fell through to the SSH library's generic "knownhosts: key mismatch"
	// text instead of the documented MITM warning. A NotEmpty-only
	// assertion passed against that generic text too, so it didn't catch
	// the bug -- this asserts the specific wording actually reaches the
	// caller.
	require.Contains(t, resp.Msg.ErrorMessage, "does not match the previously trusted key")
	require.Contains(t, resp.Msg.ErrorMessage, "MITM")
}

func TestTestRemoteConnection_MissingRemoteName_ReturnsInvalidArgument(t *testing.T) {
	svc, _, _ := newTestRemoteService(t, &config.Config{})

	_, err := svc.TestRemoteConnection(context.Background(), connect.NewRequest(&sessionv1.TestRemoteConnectionRequest{}))
	require.Error(t, err)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestTestRemoteConnection_UnknownRemote_ReturnsNotFound(t *testing.T) {
	svc, _, _ := newTestRemoteService(t, &config.Config{})

	_, err := svc.TestRemoteConnection(context.Background(), connect.NewRequest(&sessionv1.TestRemoteConnectionRequest{
		RemoteName: "does-not-exist",
	}))
	require.Error(t, err)
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestTrustRemoteHostKey_MissingFingerprint_ReturnsInvalidArgument(t *testing.T) {
	svc, _, _ := newTestRemoteService(t, &config.Config{})

	_, err := svc.TrustRemoteHostKey(context.Background(), connect.NewRequest(&sessionv1.TrustRemoteHostKeyRequest{
		RemoteName: "test-remote",
	}))
	require.Error(t, err)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestHostKeyAwareErrorMessage_UsesMITMWording_ForRawKnownHostsMismatchError
// is a low-level, no-network pin on hostKeyAwareErrorMessage itself: it
// feeds in exactly the error shape TestRemoteConnection's real dial
// produces on a mismatch (the RAW error from KnownHostsStore.HostKeyCallback(),
// NOT a *sshremote.ErrHostKeyMismatch -- that type is only ever produced by
// Verify, a different call path) and asserts the documented MITM copy comes
// out, not the SSH library's generic "knownhosts: key mismatch" text. This
// is what a prior regression got wrong: the function checked errors.As
// against *sshremote.ErrHostKeyMismatch, which this exact input never
// satisfies, so the branch never fired.
func TestHostKeyAwareErrorMessage_UsesMITMWording_ForRawKnownHostsMismatchError(t *testing.T) {
	knownHosts, err := sshremote.NewKnownHostsStore()
	require.NoError(t, err)

	host := "prod.example.com:22"
	original := newRemoteTestKey(t)
	rotated := newRemoteTestKey(t)
	require.NoError(t, knownHosts.Trust(host, original))

	// Exactly what TestRemoteConnection's ssh.ClientConfig.HostKeyCallback
	// (built from knownHosts.HostKeyCallback()) returns on a mismatch: the
	// raw *knownhosts.KeyError, untranslated.
	rawErr := knownHosts.HostKeyCallback()(host, nil, rotated)
	require.Error(t, rawErr)
	require.True(t, sshremote.IsHostKeyMismatch(rawErr), "precondition: input must actually be a mismatch error")

	msg := hostKeyAwareErrorMessage(host, rawErr)
	require.Contains(t, msg, "does not match the previously trusted key")
	require.Contains(t, msg, "MITM")

	// Sanity check the negative case too: a non-mismatch error must pass
	// through unchanged, not get the MITM copy.
	require.Equal(t, "boom", hostKeyAwareErrorMessage(host, errors.New("boom")))
}

func TestTrustRemoteHostKey_UnknownRemote_ReturnsNotFound(t *testing.T) {
	svc, _, _ := newTestRemoteService(t, &config.Config{})

	_, err := svc.TrustRemoteHostKey(context.Background(), connect.NewRequest(&sessionv1.TrustRemoteHostKeyRequest{
		RemoteName:  "does-not-exist",
		Fingerprint: "SHA256:whatever",
	}))
	require.Error(t, err)
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
