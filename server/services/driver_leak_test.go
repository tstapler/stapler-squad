package services

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session"
)

// A SessionDriver left running after its test polls every 2s, and each poll's
// config.LoadConfig() can write config.json into whichever test's
// STAPLER_SQUAD_TEST_DIR is current, racing that test's TempDir RemoveAll.

// stopSessionDriverOnCleanup stops and joins inst's SessionDriver when the test
// ends. Use it for instances that never enter the fixture's poller (e.g. built
// with session.CreateManagedInstance), which setupForkTestFixture cannot see.
func stopSessionDriverOnCleanup(t testing.TB, inst *session.Instance) {
	t.Helper()
	t.Cleanup(func() { stopSessionDriver(inst) })
}

func stopSessionDriver(inst *session.Instance) {
	session.StopSessionDriver(inst)
	session.JoinSessionDriver(inst)
}

// stopPollerSessionDrivers stops the driver of every instance registered with poller.
func stopPollerSessionDrivers(poller *session.ReviewQueuePoller) {
	for _, inst := range poller.GetInstances() {
		stopSessionDriver(inst)
	}
}

// liveSessionDriverCount counts running SessionDriver goroutines in this process.
func liveSessionDriverCount() int {
	buf := make([]byte, 1<<24)
	buf = buf[:runtime.Stack(buf, true)]
	n := 0
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "session.StartSessionDriver.func") {
			n++
		}
	}
	return n
}

// awaitNoSessionDrivers waits briefly for just-stopped drivers to exit and
// returns the count still alive.
func awaitNoSessionDrivers(timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		n := liveSessionDriverCount()
		if n == 0 || time.Now().After(deadline) {
			return n
		}
		<-time.After(50 * time.Millisecond) // no *testing.T in TestMain, so no require.Eventually
	}
}
