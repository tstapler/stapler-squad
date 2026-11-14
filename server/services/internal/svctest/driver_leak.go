package svctest

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/instance"
)

// stopSessionDriverOnCleanup stops and joins inst's SessionDriver when the test
// ends. Use it for instances that never enter the fixture's poller (e.g. built
// with session.CreateManagedInstance), which setupForkTestFixture cannot see.
func StopSessionDriverOnCleanup(t testing.TB, inst *instance.Instance) {
	t.Helper()
	t.Cleanup(func() { StopSessionDriver(inst) })
}

func StopSessionDriver(inst *instance.Instance) {
	instance.StopSessionDriver(inst)
	instance.JoinSessionDriver(inst)
}

// stopPollerSessionDrivers stops the driver of every instance registered with poller.
func StopPollerSessionDrivers(poller *session.ReviewQueuePoller) {
	for _, inst := range poller.GetInstances() {
		StopSessionDriver(inst)
	}
}

// liveSessionDriverStacks returns the stack of each running SessionDriver goroutine.
func LiveSessionDriverStacks() []string {
	buf := make([]byte, 1<<24)
	buf = buf[:runtime.Stack(buf, true)]
	var stacks []string
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "session.StartSessionDriver.func") {
			stacks = append(stacks, g)
		}
	}
	return stacks
}

// liveSessionDriverCount counts running SessionDriver goroutines in this process.
func liveSessionDriverCount() int { return len(LiveSessionDriverStacks()) }

// awaitNoSessionDrivers waits briefly for just-stopped drivers to exit and
// returns the count still alive.
func AwaitNoSessionDrivers(timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		n := liveSessionDriverCount()
		if n == 0 || time.Now().After(deadline) {
			return n
		}
		<-time.After(50 * time.Millisecond) // no *testing.T in TestMain, so no require.Eventually
	}
}

// driverStartLog remembers the creating stack of every driver started in this
// test binary, so a leak report can name the code path that started it.
var driverStartLog struct {
	mu     sync.Mutex
	starts map[*instance.Instance]string
}

func RecordSessionDriverStarts() {
	driverStartLog.starts = map[*instance.Instance]string{}
	session.SetSessionDriverStartHookForTest(func(inst *instance.Instance) {
		stack := make([]byte, 1<<14)
		stack = stack[:runtime.Stack(stack, false)]
		driverStartLog.mu.Lock()
		driverStartLog.starts[inst] = fmt.Sprintf("session %q started by:\n%s", inst.Snapshot().Title, stack)
		driverStartLog.mu.Unlock()
	})
}

// leakedSessionDriverStarts returns the creation record of each driver still running.
func LeakedSessionDriverStarts() []string {
	driverStartLog.mu.Lock()
	defer driverStartLog.mu.Unlock()
	var leaked []string
	for inst, rec := range driverStartLog.starts {
		if session.SessionDriverRunningForTest(inst) {
			leaked = append(leaked, rec)
		}
	}
	return leaked
}
