package services

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session"
)

// A SessionDriver left running after its test polls every 2s, and each poll's
// config.LoadConfig() can write config.json into whichever test's
// STAPLER_SQUAD_TEST_DIR is current, racing that test's TempDir RemoveAll.

// Tests must start drivers they own with session.StartSessionDriverForTest; the
// nosessiondrivertest analyzer rejects a raw session.StartSessionDriver.
// Drivers started by production code under test (CreateSession etc.) are
// covered by stopSessionDriverOnCleanup / setupForkTestFixture below.

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

// liveSessionDriverStacks returns the stack of each running SessionDriver goroutine.
func liveSessionDriverStacks() []string {
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
func liveSessionDriverCount() int { return len(liveSessionDriverStacks()) }

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

// driverStartLog remembers the creating stack of every driver started in this
// test binary, so a leak report can name the code path that started it.
var driverStartLog struct {
	mu     sync.Mutex
	starts map[*session.Instance]string
}

func recordSessionDriverStarts() {
	driverStartLog.starts = map[*session.Instance]string{}
	session.SetSessionDriverStartHookForTest(func(inst *session.Instance) {
		stack := make([]byte, 1<<14)
		stack = stack[:runtime.Stack(stack, false)]
		driverStartLog.mu.Lock()
		driverStartLog.starts[inst] = fmt.Sprintf("session %q started by:\n%s", inst.Snapshot().Title, stack)
		driverStartLog.mu.Unlock()
	})
}

var testFrameRE = regexp.MustCompile(`services\.(Test\w+)`)

// testFramesIn names the Test* functions on a start stack. Empty ("<none: async start>")
// when the driver was started from a goroutine the test spawned, e.g. CreateSession's
// async init; the stack's frames then show which production path did it.
func testFramesIn(stack string) string {
	var names []string
	for _, m := range testFrameRE.FindAllStringSubmatch(stack, -1) {
		names = append(names, m[1])
	}
	if len(names) == 0 {
		return "<none: async start>"
	}
	return strings.Join(names, ",")
}

// leakedSessionDriverStarts returns the creation record of each driver still running.
func leakedSessionDriverStarts() []string {
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
