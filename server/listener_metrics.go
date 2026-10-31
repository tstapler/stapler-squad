package server

// listener_metrics.go wraps the main and remote net.Listeners so accept-side
// pressure is visible: net/http's ConnState stops reporting a connection once it
// is hijacked (every WebSocket), so only the wrapped net.Conn can say how many
// accepted connections are still open and how old they are.

import (
	"bytes"
	"context"
	"errors"
	stdlog "log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

var (
	listenerInstOnce sync.Once
	listenerAccepts  metric.Int64Counter
	listenerErrors   metric.Int64Counter
	listenerAge      metric.Float64Histogram
	listenerOpenMu   sync.Mutex
	listenerOpen     = map[string]*atomic.Int64{} // by listener name; bounded to the 2 wrapped listeners
)

func initListenerMetrics() {
	listenerInstOnce.Do(func() {
		meter := telemetry.GetMeter()
		var err error
		if listenerAccepts, err = meter.Int64Counter("http.listener.accepts_total",
			metric.WithDescription("Connections accepted per listener (rate = accept throughput)")); err != nil {
			log.Warn("server: listener accepts metric", "error", err)
		}
		if listenerErrors, err = meter.Int64Counter("http.listener.accept_errors_total",
			metric.WithDescription("Accept errors per listener by kind (emfile = out of file descriptors)")); err != nil {
			log.Warn("server: listener errors metric", "error", err)
		}
		if listenerAge, err = meter.Float64Histogram("http.listener.conn_age_seconds",
			metric.WithDescription("Lifetime of accepted connections, recorded at close (long tail = WebSockets/streams)"),
			metric.WithExplicitBucketBoundaries(0.1, 1, 5, 30, 120, 600, 1800, 7200, 28800)); err != nil {
			log.Warn("server: listener age metric", "error", err)
		}
		open, err := meter.Int64ObservableGauge("http.listener.conns_open",
			metric.WithDescription("Accepted connections not yet closed per listener, including hijacked WebSockets (unlike http.server.connections_open)"))
		if err != nil {
			log.Warn("server: listener open metric", "error", err)
			return
		}
		_, _ = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
			listenerOpenMu.Lock()
			defer listenerOpenMu.Unlock()
			for name, n := range listenerOpen {
				o.ObserveInt64(open, n.Load(), metric.WithAttributes(attribute.String("listener", name)))
			}
			return nil
		}, open)
	})
}

func openCounter(name string) *atomic.Int64 {
	listenerOpenMu.Lock()
	defer listenerOpenMu.Unlock()
	n, ok := listenerOpen[name]
	if !ok {
		n = new(atomic.Int64)
		listenerOpen[name] = n
	}
	return n
}

// meteredListener counts accepts, accept errors and open connections for one listener.
type meteredListener struct {
	net.Listener
	name string
	open *atomic.Int64
}

// withListenerMetrics wraps ln; name is a bounded label ("main", "remote").
func withListenerMetrics(name string, ln net.Listener) net.Listener {
	initListenerMetrics()
	return &meteredListener{Listener: ln, name: name, open: openCounter(name)}
}

func (l *meteredListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	attr := metric.WithAttributes(attribute.String("listener", l.name))
	if err != nil {
		if listenerErrors != nil && !errors.Is(err, net.ErrClosed) {
			listenerErrors.Add(context.Background(), 1, metric.WithAttributes(
				attribute.String("listener", l.name), attribute.String("kind", acceptErrorKind(err))))
		}
		return nil, err
	}
	if listenerAccepts != nil {
		listenerAccepts.Add(context.Background(), 1, attr)
	}
	l.open.Add(1)
	return &meteredConn{Conn: c, listener: l, born: time.Now()}, nil
}

// acceptErrorKind buckets an accept error into a bounded label set.
func acceptErrorKind(err error) string {
	switch {
	case errors.Is(err, syscall.EMFILE), errors.Is(err, syscall.ENFILE):
		return "emfile"
	case errors.Is(err, os.ErrDeadlineExceeded):
		return "timeout"
	default:
		return "other"
	}
}

type meteredConn struct {
	net.Conn
	listener *meteredListener
	born     time.Time
	closed   atomic.Bool
}

func (c *meteredConn) Close() error {
	if c.closed.CompareAndSwap(false, true) {
		c.listener.open.Add(-1)
		if listenerAge != nil {
			listenerAge.Record(context.Background(), time.Since(c.born).Seconds(),
				metric.WithAttributes(attribute.String("listener", c.listener.name)))
		}
	}
	return c.Conn.Close()
}

var (
	tlsHandshakeOnce    sync.Once
	tlsHandshakeCounter metric.Int64Counter
)

// tlsHandshakeLog counts net/http's "TLS handshake error" lines (its only signal for
// failed handshakes) and passes everything through to stderr like the default logger.
type tlsHandshakeLog struct{ listener string }

func (w tlsHandshakeLog) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("TLS handshake error")) {
		tlsHandshakeOnce.Do(func() {
			var err error
			if tlsHandshakeCounter, err = telemetry.GetMeter().Int64Counter("http.listener.tls_handshake_errors_total",
				metric.WithDescription("Failed TLS handshakes per listener (bad cert, client abort, plain HTTP to the TLS port)")); err != nil {
				log.Warn("server: tls handshake metric", "error", err)
			}
		})
		if tlsHandshakeCounter != nil {
			tlsHandshakeCounter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("listener", w.listener)))
		}
	}
	return os.Stderr.Write(p)
}

func newTLSHandshakeErrorLog(listener string) *stdlog.Logger {
	return stdlog.New(tlsHandshakeLog{listener: listener}, "", stdlog.LstdFlags)
}
