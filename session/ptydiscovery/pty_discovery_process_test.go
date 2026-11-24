package ptydiscovery

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

func TestParseProcessState(t *testing.T) {
	t.Parallel()
	cases := []struct {
		state string
		want  PTYStatus
	}{
		{"", PTYError},
		{"R+", PTYBusy},
		{"D", PTYBusy},
		{"S", PTYReady},
		{"Ss", PTYReady},
		{"I", PTYReady},
		{"Z", PTYError},
		{"T", PTYIdle},
		{"X", PTYReady},
	}
	for _, tc := range cases {
		if got := parseProcessState(tc.state); got != tc.want {
			t.Errorf("parseProcessState(%q) = %v, want %v", tc.state, got, tc.want)
		}
	}
}

func TestBatchProcessStates_EmptyIsNil(t *testing.T) {
	t.Parallel()
	if got := batchProcessStates(nil); got != nil {
		t.Fatalf("batchProcessStates(nil) = %v, want nil", got)
	}
	if got := batchIsClaudeProcess(nil); got != nil {
		t.Fatalf("batchIsClaudeProcess(nil) = %v, want nil", got)
	}
}

func TestBatchProcessStates_LiveAndMissingPID(t *testing.T) {
	t.Parallel()
	cmd := safeexec.CommandContext(context.Background(), "sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start private sleep child: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// Above any kernel pid_max, so no process can ever own it: ps omits it.
	gonePID := math.MaxInt32

	// A just-forked child is briefly runnable (Busy) before it blocks in sleep.
	var got map[int]PTYStatus
	wait.RequireEventually(t, func() bool {
		got = batchProcessStates([]int{pid, gonePID})
		return got[pid] == PTYReady
	}, 5*time.Second, 10*time.Millisecond, "sleeping child never reported as Ready")
	if st := got[gonePID]; st != PTYError {
		t.Errorf("exited pid state = %v, want Error", st)
	}
}

const helperEnv = "PTYDISCOVERY_TEST_HELPER"

// TestHelperProcess_BlockForever is not a test: the claude-basename test re-executes this
// binary under a claude-like name and parks it here. It does nothing in a normal run.
func TestHelperProcess_BlockForever(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		return
	}
	select {}
}

func TestBatchIsClaudeProcess_MatchesBasenameExactly(t *testing.T) {
	t.Parallel()
	self, err := os.Executable()
	if err != nil {
		t.Skipf("cannot resolve test binary: %v", err)
	}
	// ps truncates "command" to the terminal width when COLUMNS is set, and Bazel's
	// TEST_TMPDIR is long enough to cut the basename off, so keep the path short.
	dir, err := os.MkdirTemp("/tmp", "pd")
	if err != nil {
		t.Skipf("cannot create short temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	// The children re-exec this Go test binary, which ignores argv[0]. A symlink to a
	// multicall coreutils `sleep` would dispatch on the claude-like name and exit at once.
	start := func(name string) int {
		link := filepath.Join(dir, name)
		if err := os.Symlink(self, link); err != nil {
			t.Skipf("cannot symlink: %v", err)
		}
		cmd := safeexec.CommandContext(context.Background(), link, "-test.run=^TestHelperProcess_BlockForever$")
		cmd.Env = append(os.Environ(), helperEnv+"=1")
		if err := cmd.Start(); err != nil {
			t.Skipf("cannot start %s: %v", name, err)
		}
		t.Cleanup(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})
		return cmd.Process.Pid
	}
	claudePID := start("claude")
	mixedPID := start("Claude")
	upperPID := start("CLAUDE")
	wrapperPID := start("claude-code")
	pids := []int{claudePID, mixedPID, upperPID, wrapperPID}

	// An exited child shows up as "[name] <defunct>" with no path; wait until all are alive.
	wait.RequireEventually(t, func() bool {
		for _, st := range batchProcessStates(pids) {
			if st == PTYError {
				return false
			}
		}
		return true
	}, 5*time.Second, 10*time.Millisecond, "helper children never became live (exited or zombie)")

	got := batchIsClaudeProcess(pids)
	if t.Failed() || !got[claudePID] {
		raw, _ := safeexec.CommandContext(context.Background(), "ps", "-p", joinPIDs(pids), "-o", "pid=,command=").CombinedOutput()
		t.Logf("batchIsClaudeProcess=%v; raw ps output:\n%s", got, raw)
	}
	for _, pid := range []int{claudePID, mixedPID, upperPID} {
		if !got[pid] {
			t.Errorf("pid %d running a claude-named binary (any case) should be detected", pid)
		}
	}
	if got[wrapperPID] {
		t.Errorf("pid %d running claude-code must not match (exact basename)", wrapperPID)
	}
}

func joinPIDs(pids []int) string {
	parts := make([]string, len(pids))
	for i, p := range pids {
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, ",")
}
