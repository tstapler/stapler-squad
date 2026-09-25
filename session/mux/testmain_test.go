package mux

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/testutil/tmuxreap"
)

func TestMain(m *testing.M) {
	tmuxreap.ReapLeakedTestServers()
	tmuxreap.StartTestServerWatchdog(os.Getpid())

	// Periodic goroutine dump so hangs produce visible output rather than silence.
	// all=false (current goroutine's stack only) -- the stop-the-world all=true dump
	// briefly pauses every goroutine in the process, which perturbs the subprocess
	// timing that tests like TestWriteReadUserOptions and
	// TestScanFromUserOptions_RegistersSession depend on.
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				dumpGoroutines("periodic", false)
			case <-stop:
				return
			}
		}
	}()

	// Also dump on interrupt so manual Ctrl-C reveals the hang site. This is the
	// stop-the-world hang-diagnostic path, so it keeps all=true.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		dumpGoroutines("signal", true)
	}()

	code := m.Run()
	close(stop)
	os.Exit(code)
}

func dumpGoroutines(reason string, all bool) {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, all)
	fmt.Fprintf(os.Stderr, "\n=== goroutine dump (%s) ===\n%s\n", reason, buf[:n])
}
