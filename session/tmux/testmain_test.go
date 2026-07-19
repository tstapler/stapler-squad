//go:build !windows

package tmux

import (
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/testutil/tmuxreap"
)

func TestMain(m *testing.M) {
	// Intercepted before any other setup so a re-exec'd helper process (see
	// TestExecGate_CrossProcess in exec_gate_test.go) never has to pay for the
	// reaper/watchdog startup cost of a normal test run.
	if os.Getenv(execGateCrossProcessHelperEnvVar) == "1" {
		runExecGateCrossProcessHelper()
		return
	}
	tmuxreap.ReapLeakedTestServers()
	tmuxreap.StartTestServerWatchdog(os.Getpid())
	os.Exit(m.Run())
}
