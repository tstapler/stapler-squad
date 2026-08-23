// Package services_test (an external test package, not services) is
// required here because this test exercises server.StartRemote and
// server.NewServerWithDeps directly -- the server package already imports
// server/services, so an internal (package services) test file in this
// directory cannot import server without creating a build cycle.
package services_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// findFreeTestPort asks the OS for an ephemeral port and immediately releases
// it -- mirrors server/server_integration_test.go's findFreePort, duplicated
// locally because that helper is unexported in package server and this file
// must live in package services_test (see the package doc comment above).
func findFreeTestPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// buildSelfSignedTLSFixture generates a single self-signed leaf certificate
// (acting as its own trust root) for hostnames, and returns a server-side
// *tls.Config alongside the client-side *x509.CertPool that trusts it.
//
// This deliberately duplicates the shape of server/tls.go's
// generateCA/generateServerCert (used by the Story 1.1.1 test in
// server/server_integration_test.go) rather than calling those helpers
// directly: they are unexported in package server, and this file cannot be
// package server itself (see the package doc comment above), so cross-package
// reuse isn't possible without a production code change this task doesn't
// authorize.
func buildSelfSignedTLSFixture(t *testing.T, hostnames []string) (*tls.Config, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	var dnsNames []string
	var ipAddrs []net.IP
	for _, h := range hostnames {
		if ip := net.ParseIP(h); ip != nil {
			ipAddrs = append(ipAddrs, ip)
		} else {
			dnsNames = append(dnsNames, h)
		}
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "stapler-squad-native-streaming-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(2 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddrs,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	pool := x509.NewCertPool()
	pool.AddCert(cert)

	tlsCert := tls.Certificate{Certificate: [][]byte{certDER}, PrivateKey: key}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}, pool
}

// protoCapturingTransport wraps an http.RoundTripper and records the
// protocol (e.g. "HTTP/2.0") of the most recent response, so the test can
// assert the ConnectRPC client actually spoke HTTP/2 end to end rather than
// silently falling back to HTTP/1.1.
type protoCapturingTransport struct {
	inner     http.RoundTripper
	lastProto atomic.Value // string
}

func (t *protoCapturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if resp != nil {
		t.lastProto.Store(resp.Proto)
	}
	return resp, err
}

// sessionIDFromEvent extracts the session ID from whichever oneof branch a
// SessionEvent carries, for the created/updated event types this test uses.
func sessionIDFromEvent(ev *sessionv1.SessionEvent) string {
	if c := ev.GetSessionCreated(); c != nil {
		return c.GetSession().GetId()
	}
	if u := ev.GetSessionUpdated(); u != nil {
		return u.GetSession().GetId()
	}
	return ""
}

// TestWatchSessions_should_DeliverMultipleEventsOverNativeHTTP2Stream_When_CalledThroughStartRemoteTLSListener
// is Task 1.1.1c (project_plans/web-transport-architecture-review): the
// end-to-end test both the architecture review ("give ws_stream_bridge.go its
// first-ever automated test... asserts multiple incrementally-flushed
// messages arrive") and the adversarial review (no automated e2e coverage of
// the native-HTTP/2 path) explicitly recommend. It starts a real *server.Server
// via StartRemote with a self-signed TLS fixture, drives a real ConnectRPC Go
// client configured with golang.org/x/net/http2's Transport (mirroring what
// createConnectTransport does in-browser), and confirms WatchSessions
// delivers multiple session mutations, in order, over a genuine native-HTTP/2
// stream -- not the WS-bridge path (proven by never sending a WebSocket
// upgrade request at all; StreamingWSBridge.Handler forwards any non-upgrade
// request straight to the wrapped Connect handler, see ws_stream_bridge_test.go).
func TestWatchSessions_should_DeliverMultipleEventsOverNativeHTTP2Stream_When_CalledThroughStartRemoteTLSListener(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	deps, err := server.BuildDependencies()
	require.NoError(t, err)

	srv := server.NewServerWithDeps("localhost:0", deps)

	tlsCfg, caPool := buildSelfSignedTLSFixture(t, []string{"127.0.0.1", "localhost"})

	srvCtx, srvCancel := context.WithCancel(context.Background())
	defer srvCancel()

	port := findFreeTestPort(t)
	remoteAddr := fmt.Sprintf("127.0.0.1:%d", port)
	require.NoError(t, srv.StartRemote(srvCtx, remoteAddr, tlsCfg, nil))

	rt := &protoCapturingTransport{inner: &http2.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    caPool,
			ServerName: "127.0.0.1",
		},
	}}
	httpClient := &http.Client{Transport: rt}

	// The Connect handler is mounted under "/api" (server.go:416-429) --
	// StreamingWSBridge.Handler("/api") strips this prefix before forwarding
	// to the wrapped handler, so the client must include it.
	baseURL := fmt.Sprintf("https://%s/api", remoteAddr)
	client := sessionv1connect.NewSessionServiceClient(httpClient, baseURL)

	callCtx, callCancel := context.WithCancel(context.Background())
	defer callCancel()

	// connect-go's streaming handler (protocol_connect.go's
	// connectStreamingHandlerConn.Send) only flushes response headers on the
	// handler's FIRST Send() call -- WatchSessions never sends anything until
	// either the initial snapshot (empty here: no sessions exist in this
	// isolated test dir) or the first live event. So the client's Do() call
	// blocks until this test publishes something, meaning client.WatchSessions
	// itself must run in its own goroutine here -- otherwise nothing could
	// ever publish the event that unblocks it.
	type recv struct {
		ev  *sessionv1.SessionEvent
		err error
	}
	recvCh := make(chan recv, 16)
	streamErrCh := make(chan error, 1)
	go func() {
		stream, err := client.WatchSessions(callCtx, connect.NewRequest(&sessionv1.WatchSessionsRequest{}))
		streamErrCh <- err
		if err != nil {
			return
		}
		for stream.Receive() {
			recvCh <- recv{ev: stream.Msg()}
		}
		recvCh <- recv{err: stream.Err()}
	}()

	// awaitEvent drains recvCh, ignoring any event whose session ID doesn't
	// match id (duplicate re-publishes and unrelated events), until it finds
	// one or times out.
	awaitEvent := func(id string, timeout time.Duration) *sessionv1.SessionEvent {
		t.Helper()
		deadlineCh := time.After(timeout)
		for {
			select {
			case r := <-recvCh:
				require.NoError(t, r.err)
				if sessionIDFromEvent(r.ev) == id {
					return r.ev
				}
			case <-deadlineCh:
				t.Fatalf("timed out waiting for a SessionEvent with id=%q", id)
				return nil
			}
		}
	}

	inst1 := &session.Instance{
		ID: "watch-native-stream-inst-1", Title: "watch-native-stream-inst-1",
		Path: t.TempDir(), Status: session.Active, Program: "claude",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	inst2 := &session.Instance{
		ID: "watch-native-stream-inst-2", Title: "watch-native-stream-inst-2",
		Path: t.TempDir(), Status: session.Active, Program: "claude",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	// EventBus.Publish is best-effort fan-out to already-registered
	// subscribers only, and there's an inherent race between WatchSessions
	// subscribing server-side and this goroutine's first publish -- so
	// re-publish the first mutation on a ticker until it's actually observed,
	// which proves the subscription is live before publishing the second
	// mutation exactly once.
	publishCtx, stopPublishing := context.WithCancel(context.Background())
	go func() {
		deps.EventBus.Publish(events.NewSessionCreatedEvent(inst1))
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-publishCtx.Done():
				return
			case <-ticker.C:
				deps.EventBus.Publish(events.NewSessionCreatedEvent(inst1))
			}
		}
	}()

	select {
	case err := <-streamErrCh:
		require.NoError(t, err, "expected WatchSessions to connect over native HTTP/2")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for WatchSessions to establish (response headers never flushed)")
	}

	firstEvent := awaitEvent(inst1.ID, 10*time.Second)
	stopPublishing()

	deps.EventBus.Publish(events.NewSessionUpdatedEvent(inst2, []string{"status"}))
	secondEvent := awaitEvent(inst2.ID, 5*time.Second)

	assert.NotNil(t, firstEvent.GetSessionCreated(), "expected the first mutation to arrive as a SessionCreated event")
	assert.NotNil(t, secondEvent.GetSessionUpdated(), "expected the second mutation to arrive as a SessionUpdated event")

	assert.Equal(t, "HTTP/2.0", rt.lastProto.Load(),
		"expected the ConnectRPC client to speak native HTTP/2 through StartRemote's TLS listener")
}
