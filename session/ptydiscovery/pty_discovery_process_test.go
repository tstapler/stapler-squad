package ptydiscovery

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
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

	// A reaped child's PID is the "missing" PID: ps omits it, so it must map to PTYError.
	gone := safeexec.CommandContext(context.Background(), "true")
	if err := gone.Run(); err != nil {
		t.Skipf("cannot run short-lived child: %v", err)
	}
	gonePID := gone.Process.Pid

	got := batchProcessStates([]int{pid, gonePID})
	if st := got[pid]; st != PTYReady {
		t.Errorf("sleeping child state = %v, want Ready", st)
	}
	if st := got[gonePID]; st != PTYError {
		t.Errorf("exited pid state = %v, want Error", st)
	}
}

func TestBatchIsClaudeProcess_MatchesBasenameExactly(t *testing.T) {
	t.Parallel()
	sleepBin, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not on PATH")
	}
	dir := t.TempDir()
	start := func(name string) int {
		link := filepath.Join(dir, name)
		if err := os.Symlink(sleepBin, link); err != nil {
			t.Skipf("cannot symlink: %v", err)
		}
		cmd := safeexec.CommandContext(context.Background(), link, "30")
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
	wrapperPID := start("claude-code")

	got := batchIsClaudeProcess([]int{claudePID, wrapperPID})
	if !got[claudePID] {
		t.Errorf("pid %d running ./claude should be detected", claudePID)
	}
	if got[wrapperPID] {
		t.Errorf("pid %d running claude-code must not match (exact basename)", wrapperPID)
	}
}
