package remote

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/sshremote"
	"github.com/tstapler/stapler-squad/session/sshtransport"
)

func TestHostWithPort_DefaultPortFoldsAway(t *testing.T) {
	t.Parallel()
	require.Equal(t, "h", hostWithPort("h", 0))
	require.Equal(t, "h", hostWithPort("h", 22))
	require.Equal(t, "h:2200", hostWithPort("h", 2200))
}

func TestSplitHostPort_PortRangeEdges(t *testing.T) {
	t.Parallel()
	h, p := splitHostPort("h:65535")
	require.Equal(t, "h", h)
	require.Equal(t, int32(65535), p)
	h, p = splitHostPort("h:65536")
	require.Equal(t, "h:65536", h)
	require.Zero(t, p)
}

func TestRemoteAddr(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"example.com":      "example.com:22",
		"example.com:2222": "example.com:2222",
		"10.0.0.5":         "10.0.0.5:22",
		"[::1]:2200":       "[::1]:2200",
		"::1":              "[::1]:22",
	}
	for in, want := range cases {
		require.Equal(t, want, RemoteAddr(in), in)
	}
}

func requireConnectCode(t *testing.T, err error, code connect.Code) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, code, connect.CodeOf(err), err.Error())
}

func TestGenerateRemoteIdentity_EmptyName_ReturnsInvalidArgument(t *testing.T) {
	svc, _, _ := newTestRemoteService(t, &config.Config{})
	_, err := svc.GenerateRemoteIdentity(context.Background(),
		connect.NewRequest(&sessionv1.GenerateRemoteIdentityRequest{}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)
	require.Contains(t, err.Error(), "name is required")
}

func TestCreateRemote_Validation_ReturnsInvalidArgument(t *testing.T) {
	svc, _, _ := newTestRemoteService(t, &config.Config{})
	_, err := svc.CreateRemote(context.Background(), connect.NewRequest(&sessionv1.CreateRemoteRequest{}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)
	require.Contains(t, err.Error(), "name is required")

	for _, req := range []*sessionv1.CreateRemoteRequest{
		{Name: "n", User: "u", BasePath: "/b"},
		{Name: "n", Host: "h", BasePath: "/b"},
		{Name: "n", Host: "h", User: "u"},
	} {
		_, err = svc.CreateRemote(context.Background(), connect.NewRequest(req))
		requireConnectCode(t, err, connect.CodeInvalidArgument)
		require.Contains(t, err.Error(), "host, user, and base_path are required")
	}
}

func TestDeleteRemote_EmptyName_ReturnsInvalidArgument(t *testing.T) {
	svc, _, _ := newTestRemoteService(t, &config.Config{})
	_, err := svc.DeleteRemote(context.Background(), connect.NewRequest(&sessionv1.DeleteRemoteRequest{}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)
	require.Contains(t, err.Error(), "name is required")
}

func TestResolveRemoteTarget_Branches(t *testing.T) {
	cfg := &config.Config{Remotes: []config.RemoteConfig{{Name: "r1", Host: "h1", User: "u1", IdentityRef: "id1"}}}
	svc, _, _ := newTestRemoteService(t, cfg)

	got, err := svc.resolveRemoteTarget("r1", nil)
	require.NoError(t, err)
	require.Equal(t, &resolvedRemoteTarget{name: "r1", hostForTrust: "h1", addr: "h1:22", user: "u1", identityRef: "id1"}, got)

	got, err = svc.resolveRemoteTarget("", &sessionv1.DraftRemoteTarget{Name: "d", Host: "dh", User: "du", Port: 2200})
	require.NoError(t, err)
	require.Equal(t, &resolvedRemoteTarget{name: "d", hostForTrust: "dh:2200", addr: "dh:2200", user: "du", identityRef: "d"}, got)

	for _, draft := range []*sessionv1.DraftRemoteTarget{
		{Host: "h", User: "u"}, {Name: "n", User: "u"}, {Name: "n", Host: "h"},
	} {
		_, err = svc.resolveRemoteTarget("", draft)
		requireConnectCode(t, err, connect.CodeInvalidArgument)
		require.Contains(t, err.Error(), "draft.name, draft.host, and draft.user are required")
	}

	_, err = svc.resolveRemoteTarget("", nil)
	requireConnectCode(t, err, connect.CodeInvalidArgument)
	require.Contains(t, err.Error(), "one of remote_name or draft is required")
}

func TestTrustRemoteHostKey_Unreachable_ReportsErrorMessageNotRPCError(t *testing.T) {
	cfg := &config.Config{Remotes: []config.RemoteConfig{{Name: "gone", Host: "127.0.0.1:1", User: "u"}}}
	svc, knownHosts, _ := newTestRemoteService(t, cfg)

	resp, err := svc.TrustRemoteHostKey(context.Background(), connect.NewRequest(&sessionv1.TrustRemoteHostKeyRequest{
		RemoteName: "gone", Fingerprint: "SHA256:x",
	}))
	require.NoError(t, err)
	require.False(t, resp.Msg.Success)
	require.Contains(t, resp.Msg.ErrorMessage, "could not reach 127.0.0.1:1 to verify its host key")
	trusted, err := knownHosts.IsHostTrusted("127.0.0.1:1")
	require.NoError(t, err)
	require.False(t, trusted)
}

func TestResolveIdentityAuthMethods_NoIdentity_ReturnsNil(t *testing.T) {
	_, _, keyStore := newTestRemoteService(t, &config.Config{})
	require.Nil(t, ResolveIdentityAuthMethods(context.Background(), keyStore, ""))
	require.Nil(t, ResolveIdentityAuthMethods(context.Background(), keyStore, "never-generated"))

	_, err := keyStore.GenerateOrDescribeIdentity(context.Background(), "present")
	require.NoError(t, err)
	require.Len(t, ResolveIdentityAuthMethods(context.Background(), keyStore, "present"), 1)
}

type noopPublisher struct{}

func (noopPublisher) PublishRemoteHealthChanged(string, sshremote.RemoteConnectionState, sshremote.RemoteConnectionState) {
}

func TestBuildRemoteHealthProber_BuildsProberForRemote(t *testing.T) {
	_, knownHosts, keyStore := newTestRemoteService(t, &config.Config{})
	keyring.MockInit()
	prober, err := BuildRemoteHealthProber(context.Background(),
		&config.RemoteConfig{Name: "r", Host: "127.0.0.1", User: "u", IdentityRef: "r"},
		sshtransport.NewSSHClientPool(), knownHosts, keyStore, noopPublisher{})
	require.NoError(t, err)
	require.NotNil(t, prober)
}
