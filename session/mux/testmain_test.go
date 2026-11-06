package mux

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"testing"

	"github.com/tstapler/stapler-squad/testutil/tmuxreap"
)

func TestMain(m *testing.M) {
	tmuxreap.ReapLeakedTestServers()
	tmuxreap.StartTestServerWatchdog(os.Getpid())

	// A periodic stop-the-world (all=true) dump used to run here so hangs produce
	// visible output rather than silence, but it briefly paused every goroutine in
	// the process every 30s, perturbing the subprocess timing that tests like
	// TestWriteReadUserOptions and TestScanFromUserOptions_RegistersSession depend
	// on. Dropped rather than switched to all=false: a single-goroutine dump would
	// only ever show this idle ticker's own stack, never the actual hang site,
	// which is actively misleading (looks like a diagnostic that's silently
	// useless). go test's own -timeout already stops the world and dumps every
	// goroutine when the whole binary hangs, so that case is still covered.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		dumpGoroutines("signal", true)
	}()

	code := m.Run()
	os.Exit(code)
}

func dumpGoroutines(reason string, all bool) {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, all)
	fmt.Fprintf(os.Stderr, "\n=== goroutine dump (%s) ===\n%s\n", reason, buf[:n])
}
