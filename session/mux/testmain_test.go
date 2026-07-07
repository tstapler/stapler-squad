package mux

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// Periodic goroutine dump so hangs produce visible output rather than silence.
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				dumpGoroutines("periodic")
			case <-stop:
				return
			}
		}
	}()

	// Also dump on interrupt so manual Ctrl-C reveals the hang site.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		dumpGoroutines("signal")
	}()

	code := m.Run()
	close(stop)
	os.Exit(code)
}

func dumpGoroutines(reason string) {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	fmt.Fprintf(os.Stderr, "\n=== goroutine dump (%s) ===\n%s\n", reason, buf[:n])
}
