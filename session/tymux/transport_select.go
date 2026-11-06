package tymux

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/net/http2"

	"github.com/tstapler/tymux/clients/go/selecting"
	"github.com/tstapler/tymux/clients/go/udsdialer"

	"github.com/tstapler/stapler-squad/log"
)

// transportProbeTimeout bounds each GetServerInfo probe the selector makes on its failure and
// re-probe paths (upstream applies one bound to both).
var transportProbeTimeout = 3 * time.Second

var (
	transportActiveGauge = mustInt64Gauge("tymux_transport",
		metric.WithDescription("1 for the transport (tcp|unix) squad's tymux client currently uses to reach the tymuxd at daemon (its TCP address), 0 for the other"))
	transportTransitionCounter = mustInt64Counter("tymux_transport_transitions_total",
		metric.WithDescription("Switches of squad's tymux client between the TCP and Unix-socket transports, by daemon and from/to"))
)

// recordTransportActive publishes the active transport for one daemon; the gauge is keyed by the
// daemon's address so a second daemon's selector cannot overwrite it.
func recordTransportActive(daemon string, active selecting.Kind) {
	for _, kind := range []selecting.Kind{selecting.TCP, selecting.Unix} {
		v := int64(0)
		if kind == active {
			v = 1
		}
		transportActiveGauge.Record(context.Background(), v,
			metric.WithAttributes(attribute.String("daemon", daemon), attribute.String("transport", string(kind))))
	}
}

// ErrTymuxdSocketAccessDenied means the kernel refused connect() on tymuxd's Unix control socket
// (EACCES/EPERM): a daemon is listening and does not authorize this user. It is a hard error,
// never a reason to fall back to the unauthenticated TCP listener.
var ErrTymuxdSocketAccessDenied = selecting.ErrSocketAccessDenied

// addrLogger tags the selector's own transition logs with the daemon they concern.
type addrLogger struct{ addr string }

func (l addrLogger) Debug(msg string, args ...any) {
	log.Debug("[tymux] "+msg, append(args, "addr", l.addr)...)
}

func (l addrLogger) Warn(msg string, args ...any) {
	log.Warn("[tymux] "+msg, append(args, "addr", l.addr)...)
}

// newSelector builds the Unix-first selector for the daemon at addr whose control socket is sock.
// quiet is for one-shot probes: they leave the process-global tymux_transport gauge, transition
// counter and Warn log to the shared session selector. release drops the pooled connections.
func newSelector(addr, sock string, quiet bool) (c *selecting.Client, release func()) {
	tcpHTTP, unixHTTP := udsdialer.DialTCPHTTPClient(), newUnixHTTPClient(sock)
	opts := selecting.Options{Quiet: quiet, Logger: addrLogger{addr}, ProbeTimeout: transportProbeTimeout}
	if !quiet {
		opts.OnTransition = func(from, to selecting.Kind, _ string) {
			recordTransportActive(addr, to)
			transportTransitionCounter.Add(context.Background(), 1, metric.WithAttributes(
				attribute.String("daemon", addr), attribute.String("from", string(from)), attribute.String("to", string(to))))
		}
	}
	c = selecting.NewFromTransports(
		selecting.Wrap(newTymuxClient(unixHTTP, "http://unix")),
		selecting.Wrap(newTymuxClient(tcpHTTP, addr)),
		opts)
	return c, func() {
		unixHTTP.CloseIdleConnections()
		tcpHTTP.CloseIdleConnections()
	}
}

// newUnixHTTPClient dials the control socket through upstream's trust check (path, owner and, on
// Linux, peer uid) so nothing is sent to a socket another user could have planted.
func newUnixHTTPClient(sock string) *http.Client {
	dial := selecting.CheckedDial(sock, nil)
	return &http.Client{Transport: &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, _, _ string, _ *tls.Config) (net.Conn, error) {
			conn, err := dial(ctx, "unix", sock, 0)
			return conn, refusedIfSocketPathBroken("unix", err)
		},
	}}
}

// refusedIfSocketPathBroken makes an undialable socket path (EINVAL, ENOTDIR, ENAMETOOLONG, ELOOP)
// read as ENOENT, so upstream's selector falls back to TCP and liveness reports "no socket"
// instead of treating it as an inconclusive dial.
func refusedIfSocketPathBroken(network string, err error) error {
	if network == "unix" && socketPathBroken(err) {
		return fmt.Errorf("control socket path is unusable: %w: %w", syscall.ENOENT, err)
	}
	return err
}

// accessDeniedDialError tells liveness.Probe a kernel EACCES/EPERM is a permission refusal by a
// live daemon (liveness only recognizes errors that declare themselves one).
type accessDeniedDialError struct{ error }

func (e accessDeniedDialError) Unwrap() error      { return e.error }
func (e accessDeniedDialError) AccessDenied() bool { return true }

func markAccessDenied(err error) error {
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return accessDeniedDialError{err}
	}
	return err
}

// socketPathForAddr is the Unix control socket that belongs to the daemon at addr, or "" when it
// cannot be known. It only ever returns the socket of the daemon this process itself resolves
// (ResolveDaemonConfig: STAPLER_SQUAD_INSTANCE-scoped) or the shared default daemon's; an addr
// that is neither has no socket, so another instance's daemon is never used.
func socketPathForAddr(addr string) string {
	if rc := ResolveDaemonConfig(); rc.Addr == addr {
		return unixSocketPath(rc)
	}
	return unixSocketPath(DaemonConfig{Addr: addr})
}

// newUnixTransport is the single-endpoint Unix transport the supervisor probes, with no TCP
// fallback to mask a dead socket.
func newUnixTransport(socketPath string) (selecting.Transport, *http.Client) {
	hc := newUnixHTTPClient(socketPath)
	return selecting.Wrap(newTymuxClient(hc, "http://unix")), hc
}

// newTCPTransport is the TCP-only transport; probeDaemon and the monitor's TCP-liveness check use
// it directly so the Unix socket cannot answer for the TCP listener.
func newTCPTransport(addr string) (selecting.Transport, *http.Client) {
	hc := udsdialer.DialTCPHTTPClient()
	return selecting.Wrap(newTymuxClient(hc, addr)), hc
}
