//go:build unix

package telemetry

import (
	"context"
	"os"
	"sync"
	"syscall"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
)

var fdMetricsOnce sync.Once

func init() {
	fdMetricsOnce.Do(func() {
		if err := registerFDMetrics(); err != nil {
			log.Warn("telemetry: failed to register fd metrics", "error", err)
		}
	})
}

// openFDCount counts entries in /dev/fd (present on Linux and macOS). The
// directory handle used for the listing is itself one entry, so it is subtracted.
func openFDCount() (int, bool) {
	// Readdirnames avoids os.ReadDir's per-entry lstat, which fails on /dev/fd entries.
	dir, err := os.Open("/dev/fd")
	if err != nil {
		return 0, false
	}
	defer dir.Close()
	names, err := dir.Readdirnames(-1)
	if err != nil || len(names) == 0 {
		return 0, false
	}
	return len(names) - 1, true
}

func fdSoftLimit() (uint64, bool) {
	var rl syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rl); err != nil {
		return 0, false
	}
	return uint64(rl.Cur), true //nolint:gosec,unconvert // Rlimit.Cur width differs per platform
}

func registerFDMetrics() error {
	meter := GetMeter()
	open, err := meter.Int64ObservableGauge("process.open_fds", metric.WithDescription("Open file descriptors of this process"))
	if err != nil {
		return err
	}
	limit, err := meter.Int64ObservableGauge("process.fd_limit", metric.WithDescription("Soft RLIMIT_NOFILE; open_fds approaching it means accept() starts failing with EMFILE"))
	if err != nil {
		return err
	}
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		if n, ok := openFDCount(); ok {
			o.ObserveInt64(open, int64(n))
		}
		if l, ok := fdSoftLimit(); ok {
			o.ObserveInt64(limit, int64(l)) //nolint:gosec // limit fits int64 except RLIM_INFINITY, which is not actionable
		}
		return nil
	}, open, limit)
	return err
}
