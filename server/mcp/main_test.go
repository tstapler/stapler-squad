package mcp

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/instance"
)

// TestMain fails the run if a test leaves a SessionDriver running: the driver
// re-reads config.json from the ambient state dir, so one that outlives its test
// writes into a later test's TempDir ("RemoveAll cleanup: directory not empty").
func TestMain(m *testing.M) {
	var mu sync.Mutex
	var started []*instance.Instance
	session.SetSessionDriverStartHookForTest(func(inst *instance.Instance) {
		mu.Lock()
		started = append(started, inst)
		mu.Unlock()
	})
	code := m.Run()

	mu.Lock()
	defer mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for {
		leaked := 0
		for _, inst := range started {
			if session.SessionDriverRunningForTest(inst) {
				leaked++
			}
		}
		if leaked == 0 {
			break
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(os.Stderr, "FAIL: %d SessionDriver goroutine(s) outlived their tests; stop them in t.Cleanup (see newWorktreeGuardHandlers)\n", leaked)
			code = 1
			break
		}
		<-time.After(20 * time.Millisecond) // no *testing.T in TestMain, so no require.Eventually
	}
	os.Exit(code)
}
