package services

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"strings"
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
// KnownHostsStore/KeyStore (both test-isolated: KnownHostsStore via a
// per-test STAPLER_SQUAD_TEST_DIR, KeyStore via go-keyring's mock backend)
// and a fixed config.Config.
func newTestRemoteService(t *testing.T, cfg *config.Config) (*RemoteService, *sshremote.KnownHostsStore, *sshremote.KeyStore) {
	t.Helper()
	// config.GetConfigDirForDir's IsTestMode() auto-isolation keys only on
	// PID, so every test in this binary shares one known_hosts file by
	// default. That's not test-isolated across tests in this file: each
	// test's throwaway SSH server (startRemoteTestSSHServer) binds an
	// OS-assigned ephemeral port, and a later test can be handed a port an
	// earlier test already trusted a *different* host key for once the OS
	// recycles it -- e.g. TestTestRemoteConnection_ReportsMismatch_...
	// intentionally trusts a stale key for its addr and expects
	// TestRemoteConnection to reject it, which silently passes for the
	// wrong reason if that addr already carried a real trusted entry left
	// over from another test. A per-test temp dir removes the shared file.
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
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

// ─── Draft-mode TestRemoteConnection/TrustRemoteHostKey (Phase 6, Epic 6.1) ─
//
// The Add Remote form tests/trusts a remote BEFORE it's saved to
// config.json (see ux.md Surface 2/3's interaction flow and this feature's
// AC7: "the remote is not persisted until Trust and connect is clicked").
// These exercise the `draft` field added to both requests for that flow,
// as opposed to the `remote_name` path exercised above.

func TestTestRemoteConnection_Draft_ReturnsHostKeyUnknown_When_NeverSeen(t *testing.T) {
	addr, hostKey := startRemoteTestSSHServer(t)
	svc, _, _ := newTestRemoteService(t, &config.Config{})

	resp, err := svc.TestRemoteConnection(context.Background(), connect.NewRequest(&sessionv1.TestRemoteConnectionRequest{
		Draft: &sessionv1.DraftRemoteTarget{Name: "draft-remote", Host: addr, User: "testuser"},
	}))
	require.NoError(t, err)

	require.False(t, resp.Msg.Success)
	require.True(t, resp.Msg.HostKeyUnknown)
	require.Equal(t, sshremote.HostKeyFingerprint(hostKey), resp.Msg.Fingerprint)
}

func TestDraftFlow_GenerateIdentity_TrustHostKey_Then_CreateRemote_RoundTripSucceeds(t *testing.T) {
	addr, hostKey := startRemoteTestSSHServer(t)
	cfg := &config.Config{}
	svc, knownHosts, keyStore := newTestRemoteService(t, cfg)
	ctx := context.Background()

	// 1. Generate the identity the Add Remote form would show as the
	// authorized_keys line -- BEFORE anything is saved to config.
	genResp, err := svc.GenerateRemoteIdentity(ctx, connect.NewRequest(&sessionv1.GenerateRemoteIdentityRequest{Name: "draft-remote"}))
	require.NoError(t, err)
	require.NotEmpty(t, genResp.Msg.PublicKeyText)
	require.Contains(t, genResp.Msg.AuthorizedKeysLine, genResp.Msg.PublicKeyText)

	draft := &sessionv1.DraftRemoteTarget{Name: "draft-remote", Host: addr, User: "testuser"}

	// 2. First test attempt: host key unknown, remote still not in config.
	testResp, err := svc.TestRemoteConnection(ctx, connect.NewRequest(&sessionv1.TestRemoteConnectionRequest{Draft: draft}))
	require.NoError(t, err)
	require.True(t, testResp.Msg.HostKeyUnknown)
	_, stillUnsaved := cfg.RemoteByName("draft-remote")
	require.False(t, stillUnsaved, "remote must not be persisted before Trust and connect")

	// 3. Trust and connect.
	trustResp, err := svc.TrustRemoteHostKey(ctx, connect.NewRequest(&sessionv1.TrustRemoteHostKeyRequest{
		Fingerprint: testResp.Msg.Fingerprint,
		Draft:       draft,
	}))
	require.NoError(t, err)
	require.True(t, trustResp.Msg.Success, "TrustRemoteHostKey error: %s", trustResp.Msg.ErrorMessage)
	require.NoError(t, knownHosts.Verify(addr, hostKey))
	_, stillUnsavedAfterTrust := cfg.RemoteByName("draft-remote")
	require.False(t, stillUnsavedAfterTrust, "TrustRemoteHostKey alone must not persist the remote -- CreateRemote does")

	// 4. Only now does the frontend persist it.
	createResp, err := svc.CreateRemote(ctx, connect.NewRequest(&sessionv1.CreateRemoteRequest{
		Name:     "draft-remote",
		Host:     addr,
		User:     "testuser",
		BasePath: "/srv/workspaces",
	}))
	require.NoError(t, err)
	require.Equal(t, "draft-remote", createResp.Msg.Remote.Name)
	require.True(t, createResp.Msg.Remote.HasIdentity)

	saved, ok := cfg.RemoteByName("draft-remote")
	require.True(t, ok)
	require.Equal(t, "draft-remote", saved.IdentityRef)

	// 5. The identity used for CreateRemote must be the SAME one shown in
	// step 1 -- not silently rotated.
	_, privKey, err := keyStore.GetIdentity(ctx, "draft-remote")
	require.NoError(t, err)
	signer, err := ssh.ParsePrivateKey(privKey)
	require.NoError(t, err)
	require.Equal(t, genResp.Msg.PublicKeyText, strings.TrimSuffix(string(ssh.MarshalAuthorizedKey(signer.PublicKey())), "\n"))

	// 6. Saved remote can now fully connect (host trusted + identity registered).
	final, err := svc.TestRemoteConnection(ctx, connect.NewRequest(&sessionv1.TestRemoteConnectionRequest{RemoteName: "draft-remote"}))
	require.NoError(t, err)
	require.True(t, final.Msg.Success, "final TestRemoteConnection error: %s", final.Msg.ErrorMessage)
}

func TestTestRemoteConnection_RemoteNameAndDraftBothSet_ReturnsInvalidArgument(t *testing.T) {
	svc, _, _ := newTestRemoteService(t, &config.Config{})

	_, err := svc.TestRemoteConnection(context.Background(), connect.NewRequest(&sessionv1.TestRemoteConnectionRequest{
		RemoteName: "test-remote",
		Draft:      &sessionv1.DraftRemoteTarget{Name: "test-remote", Host: "example.com", User: "u"},
	}))
	require.Error(t, err)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// ─── ListRemotes / CreateRemote / DeleteRemote ──────────────────────────────

func TestCreateRemote_DuplicateName_ReturnsAlreadyExists(t *testing.T) {
	cfg := &config.Config{Remotes: []config.RemoteConfig{{Name: "prod-box", Host: "prod.example.com", User: "tyler"}}}
	svc, _, _ := newTestRemoteService(t, cfg)

	_, err := svc.CreateRemote(context.Background(), connect.NewRequest(&sessionv1.CreateRemoteRequest{
		Name: "prod-box", Host: "other.example.com", User: "tyler", BasePath: "/srv/workspaces",
	}))
	require.Error(t, err)
	require.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
}

func TestCreateRemote_NonDefaultPort_FoldedIntoHostString(t *testing.T) {
	cfg := &config.Config{}
	svc, _, _ := newTestRemoteService(t, cfg)

	resp, err := svc.CreateRemote(context.Background(), connect.NewRequest(&sessionv1.CreateRemoteRequest{
		Name: "gpu-box", Host: "10.0.1.40", User: "tyler", Port: 2222, BasePath: "/home/tyler/work",
	}))
	require.NoError(t, err)
	require.Equal(t, "10.0.1.40", resp.Msg.Remote.Host)
	require.Equal(t, int32(2222), resp.Msg.Remote.Port)

	saved, ok := cfg.RemoteByName("gpu-box")
	require.True(t, ok)
	require.Equal(t, "10.0.1.40:2222", saved.Host, "non-default port must be folded into the stored Host string")
}

func TestListRemotes_ReturnsAllConfiguredRemotes(t *testing.T) {
	cfg := &config.Config{Remotes: []config.RemoteConfig{
		{Name: "prod-box", Host: "prod.example.com", User: "tyler", BasePath: "/srv/workspaces", IdentityRef: "prod-box"},
		{Name: "gpu-box", Host: "10.0.1.40:2222", User: "tyler", BasePath: "/home/tyler/work"},
	}}
	svc, _, _ := newTestRemoteService(t, cfg)

	resp, err := svc.ListRemotes(context.Background(), connect.NewRequest(&sessionv1.ListRemotesRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Remotes, 2)

	byName := map[string]*sessionv1.RemoteConfigProto{}
	for _, r := range resp.Msg.Remotes {
		byName[r.Name] = r
	}
	require.True(t, byName["prod-box"].HasIdentity)
	require.False(t, byName["gpu-box"].HasIdentity)
	require.Equal(t, "10.0.1.40", byName["gpu-box"].Host)
	require.Equal(t, int32(2222), byName["gpu-box"].Port)
}

func TestDeleteRemote_RemovesConfigEntryAndStoredIdentity(t *testing.T) {
	cfg := &config.Config{Remotes: []config.RemoteConfig{
		{Name: "prod-box", Host: "prod.example.com", User: "tyler", BasePath: "/srv/workspaces", IdentityRef: "prod-box"},
	}}
	svc, _, keyStore := newTestRemoteService(t, cfg)
	ctx := context.Background()
	_, err := keyStore.GenerateAndStoreIdentity(ctx, "prod-box")
	require.NoError(t, err)

	_, err = svc.DeleteRemote(ctx, connect.NewRequest(&sessionv1.DeleteRemoteRequest{Name: "prod-box"}))
	require.NoError(t, err)

	_, ok := cfg.RemoteByName("prod-box")
	require.False(t, ok)
	_, _, getErr := keyStore.GetIdentity(ctx, "prod-box")
	require.ErrorIs(t, getErr, sshremote.ErrIdentityNotFound)
}

// TestDeleteRemote_OrphanedIdentityOnly_SucceedsWithoutTouchingConfig covers
// the Add Remote form's Cancel-cleanup path: an identity was generated via
// GenerateRemoteIdentity for a draft that was never saved (CreateRemote
// never called), so DeleteRemote has nothing to remove from config.Remotes
// but must still clean up the orphaned keychain entry -- and must NOT report
// NotFound just because the config side had nothing to do.
func TestDeleteRemote_OrphanedIdentityOnly_SucceedsWithoutTouchingConfig(t *testing.T) {
	cfg := &config.Config{}
	svc, _, keyStore := newTestRemoteService(t, cfg)
	ctx := context.Background()
	_, err := keyStore.GenerateAndStoreIdentity(ctx, "abandoned-draft")
	require.NoError(t, err)

	_, err = svc.DeleteRemote(ctx, connect.NewRequest(&sessionv1.DeleteRemoteRequest{Name: "abandoned-draft"}))
	require.NoError(t, err)

	_, _, getErr := keyStore.GetIdentity(ctx, "abandoned-draft")
	require.ErrorIs(t, getErr, sshremote.ErrIdentityNotFound)
}

func TestDeleteRemote_NeitherConfigNorIdentityExists_ReturnsNotFound(t *testing.T) {
	svc, _, _ := newTestRemoteService(t, &config.Config{})

	_, err := svc.DeleteRemote(context.Background(), connect.NewRequest(&sessionv1.DeleteRemoteRequest{Name: "does-not-exist"}))
	require.Error(t, err)
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
