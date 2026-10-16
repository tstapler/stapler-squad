//go:build !windows

package clihelp

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/testutil/wait"
)

func helperSpecFor(mode string, lim Limits) runSpec {
	return runSpec{
		path:     ResolvedPath(os.Args[0]),
		args:     []string{"-test.run=^$"},
		extraEnv: []string{helperEnv + "=" + mode},
		lim:      lim,
	}
}

func runHelper(t *testing.T, mode string, lim Limits) RunOutput {
	t.Helper()
	out, err := runWith(context.Background(), helperSpecFor(mode, lim))
	require.NoError(t, err)
	return out
}

// pidFrom extracts the first "pid=<n>" from output.
func pidFrom(t *testing.T, text HelpText) int {
	t.Helper()
	_, rest, ok := strings.Cut(string(text), "pid=")
	require.True(t, ok, "helper printed no pid: %q", text)
	digits := strings.TrimSpace(strings.SplitN(rest, "\n", 2)[0])
	pid, err := strconv.Atoi(digits)
	require.NoError(t, err)
	return pid
}

func TestRunWith_should_TruncateAndNotTimeOut_When_OutputExceedsCap(t *testing.T) {
	out := runHelper(t, "bigout", Limits{Timeout: wait.ScaleTimeout(5 * time.Second), MaxBytes: 256 << 10})
	assert.True(t, out.Truncated)
	assert.False(t, out.TimedOut)
	assert.Len(t, out.Text, 256<<10)
}

func TestRunWith_should_KillOnOverflowBeforeTimeout_When_ChildFloods(t *testing.T) {
	out := runHelper(t, "flood", Limits{Timeout: wait.ScaleTimeout(10 * time.Second), MaxBytes: 1024})
	assert.True(t, out.Truncated)
	assert.False(t, out.TimedOut, "overflow kills the group; the timeout must not be what ended it")
}

func TestRunWith_should_TimeOutAndKillGroup_When_ChildHangsIgnoringSIGTERM(t *testing.T) {
	start := time.Now()
	out := runHelper(t, "hang", Limits{Timeout: wait.ScaleTimeout(time.Second), MaxBytes: 1024})
	assert.True(t, out.TimedOut)
	assert.Less(t, time.Since(start), wait.ScaleTimeout(5*time.Second))
	pid := pidFrom(t, out.Text)
	wait.RequireEventually(t, func() bool { return processGone(pid) }, 5*time.Second, 10*time.Millisecond)
}

func TestRunWith_should_KillSurvivingGrandchild_When_ChildExitsNormally(t *testing.T) {
	out := runHelper(t, "orphan", Limits{Timeout: wait.ScaleTimeout(5 * time.Second), MaxBytes: 1024})
	assert.False(t, out.TimedOut)
	pid := pidFrom(t, out.Text)
	wait.RequireEventually(t, func() bool { return processGone(pid) }, 5*time.Second, 10*time.Millisecond,
		"the group is killed after Wait on the normal path")
}

func TestRunWith_should_NotLeakParentEnv_When_ParentEnvIsPoisoned(t *testing.T) {
	spec := helperSpecFor("printenv", Limits{Timeout: wait.ScaleTimeout(5 * time.Second), MaxBytes: 64 << 10})
	spec.parentEnv = []string{"GITHUB_TOKEN=leak", "ANTHROPIC_API_KEY=leak", "PATH=/poison/bin", "HOME=/real/home"}
	spec.loginPath = "/login/bin"

	out, err := runWith(context.Background(), spec)
	require.NoError(t, err)
	text := string(out.Text)

	assert.NotContains(t, text, "GITHUB_TOKEN")
	assert.NotContains(t, text, "ANTHROPIC_API_KEY")
	assert.NotContains(t, text, "leak")
	assert.NotContains(t, text, "/real/home", "HOME is a private per-probe dir, not the parent's")
	assert.Contains(t, text, "PATH=/login/bin")
	assert.Contains(t, text, "TERM=dumb")
	assert.Contains(t, text, "HOME="+os.TempDir(), "HOME arrived and lives under the temp dir")
	assert.Contains(t, text, "cwdentries=0", "cwd is empty")

	cwd := lineValue(t, text, "cwd=")
	_, statErr := os.Stat(cwd)
	assert.True(t, os.IsNotExist(statErr), "cwd removed after the run")
}

func lineValue(t *testing.T, text, prefix string) string {
	t.Helper()
	for _, l := range strings.Split(text, "\n") {
		if v, ok := strings.CutPrefix(l, prefix); ok {
			return v
		}
	}
	require.Failf(t, "missing line", "no %q in %q", prefix, text)
	return ""
}

func TestRunWith_should_RunAsSessionLeader_When_Started(t *testing.T) {
	out := runHelper(t, "sid", Limits{Timeout: wait.ScaleTimeout(5 * time.Second), MaxBytes: 1024})
	var sid, pid int
	_, err := fmtSscanf(string(out.Text), &sid, &pid)
	require.NoError(t, err)
	assert.Equal(t, pid, sid)
}

func TestRunWith_should_ReturnErrorNotSuccess_When_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runWith(ctx, helperSpecFor("hang", DefaultLimits()))
	assert.Error(t, err)

	ctx, cancel = context.WithCancel(context.Background())
	time.AfterFunc(wait.ScaleTimeout(500*time.Millisecond), cancel)
	out, err := runWith(ctx, helperSpecFor("hang", Limits{Timeout: wait.ScaleTimeout(30 * time.Second), MaxBytes: 1024}))
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, out.TimedOut)
}

func TestHelpSpec_should_UseConstantHelpArgs_When_Built(t *testing.T) {
	s := helpSpec("/bin/x", DefaultLimits())
	assert.Equal(t, []string{"--help"}, s.args)
	assert.Nil(t, s.extraEnv)
	assert.Nil(t, s.parentEnv)
}

func TestProbeEnv_should_DropEverythingButAllowlist_When_ParentHasSecrets(t *testing.T) {
	env := probeEnv([]string{"GITHUB_TOKEN=x", "ANTHROPIC_API_KEY=y", "PATH=/p"}, "", "/h")
	joined := strings.Join(env, "\n")
	assert.NotContains(t, joined, "GITHUB_TOKEN")
	assert.NotContains(t, joined, "ANTHROPIC_API_KEY")
	assert.Contains(t, env, "PATH=/p", "empty loginPath falls back to the parent PATH")
	assert.Contains(t, env, "HOME=/h")
	assert.Contains(t, probeEnv(nil, "/login", "/h"), "PATH=/login")
}

func TestRunWith_should_DropExtraEnvWithoutTestPrefix_When_Started(t *testing.T) {
	spec := helperSpecFor("printenv", Limits{Timeout: wait.ScaleTimeout(5 * time.Second), MaxBytes: 64 << 10})
	spec.extraEnv = append(spec.extraEnv, "SECRET_EXTRA=1", "CLIHELP_TEST_KEEP=1")
	out, err := runWith(context.Background(), spec)
	require.NoError(t, err)
	assert.NotContains(t, string(out.Text), "SECRET_EXTRA")
	assert.Contains(t, string(out.Text), "CLIHELP_TEST_KEEP=1")
}

func TestRun_should_ReachRealProcess_When_ExportedRunCalledWithHelp(t *testing.T) {
	out, err := Run(context.Background(), ResolvedPath(os.Args[0]), Limits{Timeout: wait.ScaleTimeout(10 * time.Second), MaxBytes: 256 << 10})
	require.NoError(t, err)
	assert.Contains(t, string(out.Text), "-test.run")
}

func fmtSscanf(text string, sid, pid *int) (int, error) {
	return fmt.Sscanf(strings.TrimSpace(text), "sid=%d pid=%d", sid, pid)
}
