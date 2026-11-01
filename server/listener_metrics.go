package server

// listener_metrics.go makes connection contention visible before the HTTP layer:
// accepts and accept errors per listener, TLS handshake failures, process file
// descriptors and the kernel listen-queue limits. Cumulative per-listener
// counters use the same package-level pattern as http_connection_metrics.go.

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	stdlog "log"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/interceptors"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Listener names are a fixed set so the label stays bounded.
const (
	listenerMain   = "main"
	listenerRemote = "remote"
)

type listenerInstruments struct {
	accepted     metric.Int64Counter
	acceptErrors metric.Int64Counter
	tlsErrors    metric.Int64Counter
}

var (
	listenerInstrumentsOnce sync.Once
	listenerInstrumentsVal  *listenerInstruments
)

func listenerMetrics() *listenerInstruments {
	listenerInstrumentsOnce.Do(func() {
		m := telemetry.GetMeter()
		var err error
		li := &listenerInstruments{}
		if li.accepted, err = m.Int64Counter("http.server.accepted_total",
			metric.WithDescription("TCP connections accepted, by listener (main|remote)")); err != nil {
			log.Error("server: register http.server.accepted_total", "error", err)
			return
		}
		if li.acceptErrors, err = m.Int64Counter("http.server.accept_errors_total",
			metric.WithDescription("Accept failures by listener and kind (closed|timeout|fd_exhausted|other); fd_exhausted means the process hit its descriptor limit")); err != nil {
			log.Error("server: register http.server.accept_errors_total", "error", err)
			return
		}
		if li.tlsErrors, err = m.Int64Counter("http.server.tls_handshake_errors_total",
			metric.WithDescription("Failed TLS handshakes, by listener")); err != nil {
			log.Error("server: register http.server.tls_handshake_errors_total", "error", err)
			return
		}
		listenerInstrumentsVal = li
	})
	return listenerInstrumentsVal
}

// meteredListener counts accepts and classifies accept errors.
type meteredListener struct {
	net.Listener
	name string
}

func meterListener(name string, ln net.Listener) net.Listener {
	return &meteredListener{Listener: ln, name: name}
}

func (l *meteredListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	li := listenerMetrics()
	if li == nil {
		return c, err
	}
	if err != nil {
		li.acceptErrors.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("listener", l.name), attribute.String("kind", acceptErrorKind(err))))
		return c, err
	}
	li.accepted.Add(context.Background(), 1, metric.WithAttributes(attribute.String("listener", l.name)))
	return c, nil
}

func acceptErrorKind(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, net.ErrClosed):
		return "closed"
	case errors.Is(err, syscall.EMFILE), errors.Is(err, syscall.ENFILE):
		return "fd_exhausted"
	case errors.As(err, &ne) && ne.Timeout():
		return "timeout"
	default:
		return "other"
	}
}

// tlsHandshakeWriter counts net/http's "TLS handshake error" log lines (the only
// place the server reports them) and forwards everything else to the app log.
type tlsHandshakeWriter struct{ name string }

func (w tlsHandshakeWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if strings.Contains(msg, "TLS handshake error") {
		if li := listenerMetrics(); li != nil {
			li.tlsErrors.Add(context.Background(), 1, metric.WithAttributes(attribute.String("listener", w.name)))
		}
		log.Debug("http server", "listener", w.name, "msg", msg) // kept for diagnosis when metrics are off
		return len(p), nil
	}
	if strings.Contains(msg, "panic serving") {
		log.Error("http server handler panic", "listener", w.name, "msg", msg)
		return len(p), nil
	}
	log.Warn("http server", "listener", w.name, "msg", msg)
	return len(p), nil
}

// serverErrorLog is installed as http.Server.ErrorLog for the named listener.
func serverErrorLog(name string) *stdlog.Logger {
	return stdlog.New(tlsHandshakeWriter{name: name}, "", 0)
}

// parseTcpExtCounter reads one named counter from /proc/net/netstat content
// ("TcpExt: <names...>" followed by "TcpExt: <values...>").
func parseTcpExtCounter(data []byte, name string) (int64, bool) {
	lines := bytes.Split(data, []byte("\n"))
	for i := 0; i+1 < len(lines); i++ {
		if !bytes.HasPrefix(lines[i], []byte("TcpExt:")) || !bytes.HasPrefix(lines[i+1], []byte("TcpExt:")) {
			continue
		}
		names, vals := strings.Fields(string(lines[i])), strings.Fields(string(lines[i+1]))
		for j := 1; j < len(names) && j < len(vals); j++ {
			if names[j] == name {
				n, err := strconv.ParseInt(vals[j], 10, 64)
				return n, err == nil
			}
		}
	}
	return 0, false
}

var osSocketOnce sync.Once

func init() {
	osSocketOnce.Do(func() {
		if err := registerOSSocketMetrics(telemetry.GetMeter()); err != nil {
			log.Error("server: register OS socket metrics", "error", err)
		}
	})
}

func registerOSSocketMetrics(m metric.Meter) error {
	fds, err := m.Int64ObservableGauge("process.open_fds",
		metric.WithDescription("Open file descriptors in this process; compare with process.fd_limit"))
	if err != nil {
		return err
	}
	limit, err := m.Int64ObservableGauge("process.fd_limit",
		metric.WithDescription("Soft RLIMIT_NOFILE; accept fails with fd_exhausted when open_fds reaches it"))
	if err != nil {
		return err
	}
	somax, err := m.Int64ObservableGauge("os.tcp.somaxconn",
		metric.WithDescription("Kernel cap on a listen backlog (kern.ipc.somaxconn / net.core.somaxconn); 0 when unreadable"))
	if err != nil {
		return err
	}
	overflows, err := m.Int64ObservableCounter("os.tcp.listen_overflows_total",
		metric.WithDescription("Kernel ListenOverflows: SYNs or ACKs dropped because a listen queue was full (Linux only; absent on macOS)"))
	if err != nil {
		return err
	}
	drops, err := m.Int64ObservableCounter("os.tcp.listen_drops_total",
		metric.WithDescription("Kernel ListenDrops: connections dropped for any listen-queue reason (Linux only; absent on macOS)"))
	if err != nil {
		return err
	}
	_, err = m.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		if n, ok := openFDCount(); ok {
			o.ObserveInt64(fds, n)
		}
		if n, ok := openFDLimit(); ok {
			o.ObserveInt64(limit, n)
		}
		if n, ok := somaxconn(); ok {
			o.ObserveInt64(somax, n)
		}
		if ov, dr, ok := listenQueueCounters(); ok {
			o.ObserveInt64(overflows, ov)
			o.ObserveInt64(drops, dr)
		}
		return nil
	}, fds, limit, somax, overflows, drops)
	return err
}

// stampArrival records when a request first reached the process, for the
// request-flow interceptor's pre-handler time. It must be the outermost handler.
func stampArrival(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(interceptors.WithArrival(r.Context(), time.Now())))
	})
}
