package telemetry

// tcp_listen.go exports kernel listen-queue pressure: connections the kernel
// dropped because the accept queue was full (the client sees a stalled connect
// or reset, and the server logs nothing). ListenOverflows/ListenDrops come from
// /proc/net/netstat and exist on Linux only; macOS has no equivalent readable
// without parsing the net.inet.tcp.stats struct or shelling out to netstat (raw
// os/exec is forbidden by norawexec), so there only the somaxconn ceiling is
// exported — check overflow there manually with `netstat -s -p tcp`.

import (
	"bufio"
	"context"
	"io"
	"strconv"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
)

var tcpListenOnce sync.Once

func init() {
	tcpListenOnce.Do(func() {
		if err := registerTCPListenMetrics(); err != nil {
			log.Warn("telemetry: failed to register tcp listen metrics", "error", err)
		}
	})
}

// parseTcpExt extracts the named TcpExt counters from /proc/net/netstat content,
// which alternates a header line and a value line per protocol.
func parseTcpExt(r io.Reader, names ...string) map[string]int64 {
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	out := map[string]int64{}
	sc := bufio.NewScanner(r)
	var header []string
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 || fields[0] != "TcpExt:" {
			header = nil
			continue
		}
		if header == nil {
			header = fields
			continue
		}
		for i, h := range header {
			if want[h] && i < len(fields) {
				if v, err := strconv.ParseInt(fields[i], 10, 64); err == nil {
					out[h] = v
				}
			}
		}
		header = nil
	}
	return out
}

func registerTCPListenMetrics() error {
	meter := GetMeter()
	overflows, err := meter.Int64ObservableCounter("net.tcp.listen_overflows_total",
		metric.WithDescription("Kernel-wide SYN/ACKs dropped because a listener's accept queue was full (Linux only)"))
	if err != nil {
		return err
	}
	drops, err := meter.Int64ObservableCounter("net.tcp.listen_drops_total",
		metric.WithDescription("Kernel-wide connections dropped before reaching accept(), superset of overflows (Linux only)"))
	if err != nil {
		return err
	}
	somax, err := meter.Int64ObservableGauge("net.tcp.somaxconn",
		metric.WithDescription("Kernel ceiling on any listener's accept-queue backlog"))
	if err != nil {
		return err
	}
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		if c, ok := readListenCounters(); ok {
			o.ObserveInt64(overflows, c["ListenOverflows"])
			o.ObserveInt64(drops, c["ListenDrops"])
		}
		if v, ok := readSomaxconn(); ok {
			o.ObserveInt64(somax, v)
		}
		return nil
	}, overflows, drops, somax)
	return err
}
