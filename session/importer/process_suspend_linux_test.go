//go:build linux

package importer

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// procState returns the /proc state letter (R, S, T, ...) of pid, or "" if unreadable.
func procState(pid int32) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	s := string(b)
	return strings.Fields(s[strings.LastIndex(s, ")")+1:])[0]
}

func waitForState(t *testing.T, pid int32, want string) {
	t.Helper()
	require.Eventually(t, func() bool { return procState(pid) == want },
		5*time.Second, 5*time.Millisecond, "pid %d never reached state %q", pid, want)
}

func TestSuspendAndResumeOriginalProcess_StopsThenContinuesChild(t *testing.T) {
	t.Parallel()
	cmd := spawnSleeper(t)
	pid := int32(cmd.Process.Pid)

	if err := SuspendOriginalProcess(pid); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	waitForState(t, pid, "T")

	if err := ResumeOriginalProcess(pid); err != nil {
		t.Fatalf("resume: %v", err)
	}
	waitForState(t, pid, "S")
}
