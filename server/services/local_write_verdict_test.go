package services

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/middleware"
)

func verdictServiceConfig(listen string, verified []string) middleware.VerdictConfig {
	return middleware.VerdictConfig{
		VerifiedHosts:  func() []string { return verified },
		ListenAddr:     func() string { return listen },
		AllowedOrigins: func() []string { return []string{"https://ui.example"} },
		LocalIPs:       func() []net.IP { return []net.IP{net.ParseIP("192.168.1.50")} },
	}
}

// stampedContext runs the real stamp middleware and returns the context a
// handler behind it would see.
func stampedContext(t *testing.T, cfg *middleware.VerdictConfig, listener string, requiresAuth bool, host string, hdr map[string]string) context.Context {
	t.Helper()
	var got context.Context
	var h http.Handler = http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Context()
	})
	if cfg != nil {
		h = WithLocalWriteVerdictConfig(*cfg)(h)
	}
	h = WithRequestRecord(listener, requiresAuth)(h)
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Host = host
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	h.ServeHTTP(httptest.NewRecorder(), r)
	require.NotNil(t, got)
	return got
}

// T-RP-56 / T-RO-34 (adapter half): the verdict adapter over the stamped record.
func TestVerdict_ShouldAllowVerifiedLanHostnameOnWildcardBindForSteerAndPruneAndRejectRebindingHostAndRefuseProxiedLoopbackCallerForReplyWhenAuthOff_AndSkipTheHostTestButKeepTheOriginTestOnTheAuthenticatedChain_WhenProfilesApplied(t *testing.T) {
	cfg := verdictServiceConfig("0.0.0.0:8543", []string{"onyx.lan"})
	stamped := func(t *testing.T, listener string, requiresAuth bool, host string, hdr map[string]string) context.Context {
		return stampedContext(t, &cfg, listener, requiresAuth, host, hdr)
	}
	const loopbackPeer, lanPeer = "127.0.0.1:50000", "192.168.1.20:50000"

	t.Run("steer and prune allow a verified LAN hostname on a wildcard bind", func(t *testing.T) {
		ctx := stamped(t, ListenerLocal, false, "onyx.lan:8543", nil)
		out, err := EvaluateLocalWrite(ctx, lanPeer, LocalWriteRebinding)
		require.NoError(t, err)
		assert.True(t, out.Allowed())
		assert.False(t, out.PeerLoopback)
		assert.False(t, out.Proxied)
	})
	t.Run("rebinding and unverified Hosts are PermissionDenied with an explicit message", func(t *testing.T) {
		for _, host := range []string{"evil.example:8543", "attacker.lan:8543", "203.0.113.9:8543"} {
			ctx := stamped(t, ListenerLocal, false, host, nil)
			out, err := EvaluateLocalWrite(ctx, loopbackPeer, LocalWriteRebinding)
			require.Error(t, err, host)
			assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
			assert.Equal(t, "host_not_allowed", out.Reason)
			assert.Contains(t, err.Error(), "not a loopback name, a verified hostname")
		}
	})
	t.Run("a local IP literal passes, a foreign Origin does not", func(t *testing.T) {
		ok := stamped(t, ListenerLocal, false, "192.168.1.50:8543", nil)
		_, err := EvaluateLocalWrite(ok, lanPeer, LocalWriteRebinding)
		require.NoError(t, err)
		bad := stamped(t, ListenerLocal, false, "onyx.lan:8543", map[string]string{"Origin": "https://evil.example"})
		_, err = EvaluateLocalWrite(bad, lanPeer, LocalWriteRebinding)
		require.Error(t, err)
	})
	t.Run("reply refuses a proxied loopback caller and a remote peer when auth is off", func(t *testing.T) {
		proxied := stamped(t, ListenerLocal, false, "localhost:8543", map[string]string{"X-Forwarded-For": "10.0.0.9"})
		out, err := EvaluateLocalWrite(proxied, loopbackPeer, LocalWriteLocalCaller)
		require.Error(t, err)
		assert.Equal(t, "proxy_header", out.Reason)
		assert.True(t, out.PeerLoopback)
		assert.True(t, out.Proxied)

		remote := stamped(t, ListenerLocal, false, "onyx.lan:8543", nil)
		out, err = EvaluateLocalWrite(remote, lanPeer, LocalWriteLocalCaller)
		require.Error(t, err)
		assert.Equal(t, "peer_not_loopback", out.Reason)

		// The steer and Prune profile record the same facts without refusing on them.
		_, err = EvaluateLocalWrite(proxied, loopbackPeer, LocalWriteRebinding)
		require.NoError(t, err)
	})
	t.Run("authenticated chain skips the Host test and keeps the Origin test", func(t *testing.T) {
		undetected := stamped(t, ListenerRemote, true, "onyx.staplerhome.internal:8444", nil)
		_, err := EvaluateLocalWrite(undetected, lanPeer, LocalWriteLocalCaller)
		require.NoError(t, err, "undetected Host with a cookie is allowed and the local-caller gate does not apply")

		ipLiteral := stamped(t, ListenerRemote, true, "100.64.0.9:8444", nil)
		_, err = EvaluateLocalWrite(ipLiteral, lanPeer, LocalWriteRebinding)
		require.NoError(t, err)

		foreign := stamped(t, ListenerRemote, true, "onyx.staplerhome.internal:8444", map[string]string{"Origin": "https://evil.example"})
		_, err = EvaluateLocalWrite(foreign, lanPeer, LocalWriteRebinding)
		require.Error(t, err)
	})
	t.Run("fails closed without a record or a config", func(t *testing.T) {
		out, err := EvaluateLocalWrite(context.Background(), loopbackPeer, LocalWriteRebinding)
		require.Error(t, err)
		assert.Equal(t, "no_request_record", out.Reason)

		out, err = EvaluateLocalWrite(stampedContext(t, nil, ListenerLocal, false, "localhost:8543", nil), loopbackPeer, LocalWriteRebinding)
		require.Error(t, err)
		assert.Equal(t, "verdict_not_configured", out.Reason)
	})
}
