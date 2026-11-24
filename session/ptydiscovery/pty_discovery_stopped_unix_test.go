//go:build !windows

package ptydiscovery

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// A running and a SIGSTOPped private child must map to different statuses per pid,
// which pins the pid and state column indexes in batchProcessStates.
func TestBatchProcessStates_StoppedAndRunningChildrenMapPerPID(t *testing.T) {
	t.Parallel()
	start := func() int {
		cmd := safeexec.CommandContext(context.Background(), "sleep", "30")
		if err := cmd.Start(); err != nil {
			t.Skipf("cannot start private sleep child: %v", err)
		}
		t.Cleanup(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})
		return cmd.Process.Pid
	}
	runningPID, stoppedPID := start(), start()
	if err := syscall.Kill(stoppedPID, syscall.SIGSTOP); err != nil {
		t.Skipf("cannot SIGSTOP child: %v", err)
	}

	var got map[int]PTYStatus
	wait.RequireEventually(t, func() bool {
		got = batchProcessStates([]int{runningPID, stoppedPID})
		return got[stoppedPID] == PTYIdle && got[runningPID] == PTYReady
	}, 5*time.Second, 10*time.Millisecond, "children never reported as stopped=Idle, running=Ready")
}
