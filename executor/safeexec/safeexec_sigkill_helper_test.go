//go:build !windows

package safeexec

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// sigkillHelperEnvVar gates the re-exec'd helper process used by
// safeexec_pg_test.go to exercise the SIGKILL-escalation path with a real
// subprocess that ignores SIGTERM — mirrors the existing
// safeexec_pdeathsig_linux_test.go re-exec convention (see pdeathsigHelperEnvVar).
const sigkillHelperEnvVar = "SAFEEXEC_SIGKILL_HELPER"

// runSigkillHelperProcess ignores SIGTERM and blocks until SIGKILLed by the
// test, forcing CommandContextPG's cmd.Cancel down the escalation path.
func runSigkillHelperProcess() {
	signal.Ignore(syscall.SIGTERM)
	fmt.Println("ready")
	os.Stdout.Sync()
	time.Sleep(time.Hour) // block until SIGKILLed by the test
}
