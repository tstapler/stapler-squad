package svctest

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// runGitTestCmd runs a git command in dir and fails the test on error.
func RunGitTestCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:norawexec,norawgitcli // test helper, blocking CombinedOutput, no zombie risk
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), out)
	return string(out)
}
