package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// TestControlMode_InterleavedConcurrentStartStop_NoDataRace is BUG-086's repro:
// many goroutines each call StartControlMode() immediately followed by
// StopControlMode() against one shared, real tmux session. This restores the
// interleaved-concurrency shape that session/instance_control_mode_ownership_test.go
// used to exercise (see its "NOT paired with an immediate concurrent
// StopControlMode" comment) before it was rewritten to call StopControlMode
// once via t.Cleanup instead -- a workaround for this bug, not a fix (per
// docs/bugs/open/BUG-086-tmux-control-mode-refcounting-race-under-concurrent-start-stop.md).
//
// Run with -race: before the fix, this reliably reported a WARNING: DATA RACE
// between processControlModeLine's write and StopControlMode's read of
// controlModeExited (both under readControlModeOutput's reader goroutine and
// the calling goroutine respectively) -- see control_mode.go's history for
// the exact field. The refcount/subscriber/cmd fields themselves are already
// guarded by controlModeSubMu; this test exists to catch a regression that
// reopens that or any adjacent unguarded field.
func TestControlMode_InterleavedConcurrentStartStop_NoDataRace(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping real tmux test")
	}

	socketName := fmt.Sprintf("test_cm_race_%d_%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = safeexec.CommandContext(ctx, Binary(), "-L", socketName, "kill-server").Run()
	})

	sessionName := "cm_race_session"
	sess := NewTmuxSessionWithServerSocket(sessionName, "sleep 60", TmuxPrefix, socketName, WithRegistry(nil))
	if err := sess.Start(t.TempDir()); err != nil {
		t.Fatalf("failed to start real tmux session: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := sess.StartControlMode(); err != nil {
				// StartControlMode can legitimately fail (e.g. tmux client/server
				// version skew disables control mode for this socket) -- that's
				// not the race under test, so don't fail the test on it. Skip the
				// paired Stop in that case, mirroring how every real caller only
				// calls StopControlMode after a successful Start.
				return
			}
			_ = sess.StopControlMode()
		}()
	}
	wg.Wait()
}
